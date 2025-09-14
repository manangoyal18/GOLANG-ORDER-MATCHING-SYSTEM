package engine

import (
	"time"

	"order-matching-engine/internal/models"

	"github.com/shopspring/decimal"
)

// MatchResult is the result of matching an incoming order against the book.
type MatchResult struct {
	Trades            []models.Trade
	UpdatedOrders     []*models.Order
	IncomingOrderLeft *models.Order // nil if fully filled
}

// Matcher implements the order matching algorithm using price-time priority.
type Matcher struct{}

// NewMatcher returns a new Matcher instance.
func NewMatcher() *Matcher { return &Matcher{} }

// Match tries to match incomingOrder against the provided orderBook.
// Returns the trades executed and any updated/resting orders. If the incoming
// limit order is not fully filled, IncomingOrderLeft will contain the leftover.
func (m *Matcher) Match(incomingOrder *models.Order, orderBook *OrderBook) *MatchResult {
	result := &MatchResult{
		Trades:        make([]models.Trade, 0),
		UpdatedOrders: make([]*models.Order, 0),
	}

	// Work on a copy so we can return the updated incoming order state.
	workingOrder := *incomingOrder
	executedAt := time.Now()

	if incomingOrder.Side == models.OrderSideBuy {
		m.matchBuyOrder(&workingOrder, orderBook, result, executedAt)
	} else {
		m.matchSellOrder(&workingOrder, orderBook, result, executedAt)
	}

	// Finalize incoming order status according to remaining quantity and type.
	if !workingOrder.RemainingQuantity.IsZero() {
		if workingOrder.Type == models.OrderTypeLimit {
			if workingOrder.RemainingQuantity.LessThan(workingOrder.InitialQuantity) {
				workingOrder.Status = models.OrderStatusPartiallyFilled
			}
			result.IncomingOrderLeft = &workingOrder
		} else {
			// Market orders: leftover is canceled when no more matches exist.
			workingOrder.Status = models.OrderStatusCanceled
			workingOrder.RemainingQuantity = decimal.Zero
			result.UpdatedOrders = append(result.UpdatedOrders, &workingOrder)
		}
	} else {
		workingOrder.Status = models.OrderStatusFilled
		result.UpdatedOrders = append(result.UpdatedOrders, &workingOrder)
	}

	return result
}

func (m *Matcher) matchBuyOrder(buyOrder *models.Order, orderBook *OrderBook, result *MatchResult, executedAt time.Time) {
	for !buyOrder.RemainingQuantity.IsZero() {
		bestAsk := orderBook.GetBestAsk()
		if bestAsk == nil {
			return
		}

		if !m.canMatch(buyOrder, bestAsk) {
			return
		}

		trade := m.executeTrade(buyOrder, bestAsk, executedAt)
		result.Trades = append(result.Trades, trade)

		// Update quantities and statuses
		tradeQuantity := trade.Quantity
		buyOrder.RemainingQuantity = buyOrder.RemainingQuantity.Sub(tradeQuantity)
		bestAsk.RemainingQuantity = bestAsk.RemainingQuantity.Sub(tradeQuantity)

		if bestAsk.RemainingQuantity.IsZero() {
			bestAsk.Status = models.OrderStatusFilled
			orderBook.RemoveOrder(bestAsk.ID, bestAsk.Side, bestAsk.Price)
		} else {
			bestAsk.Status = models.OrderStatusPartiallyFilled
		}
		bestAsk.UpdatedAt = executedAt
		result.UpdatedOrders = append(result.UpdatedOrders, bestAsk)
	}
}

func (m *Matcher) matchSellOrder(sellOrder *models.Order, orderBook *OrderBook, result *MatchResult, executedAt time.Time) {
	for !sellOrder.RemainingQuantity.IsZero() {
		bestBid := orderBook.GetBestBid()
		if bestBid == nil {
			return
		}

		if !m.canMatch(sellOrder, bestBid) {
			return
		}

		trade := m.executeTrade(sellOrder, bestBid, executedAt)
		result.Trades = append(result.Trades, trade)

		tradeQuantity := trade.Quantity
		sellOrder.RemainingQuantity = sellOrder.RemainingQuantity.Sub(tradeQuantity)
		bestBid.RemainingQuantity = bestBid.RemainingQuantity.Sub(tradeQuantity)

		if bestBid.RemainingQuantity.IsZero() {
			bestBid.Status = models.OrderStatusFilled
			orderBook.RemoveOrder(bestBid.ID, bestBid.Side, bestBid.Price)
		} else {
			bestBid.Status = models.OrderStatusPartiallyFilled
		}
		bestBid.UpdatedAt = executedAt
		result.UpdatedOrders = append(result.UpdatedOrders, bestBid)
	}
}

// canMatch returns true if incomingOrder can match restingOrder.
// Market orders match if a resting order exists; limit orders require price compatibility.
func (m *Matcher) canMatch(incomingOrder, restingOrder *models.Order) bool {
	if incomingOrder.Type == models.OrderTypeMarket {
		return true
	}
	if incomingOrder.Price == nil || restingOrder.Price == nil {
		return false
	}
	if incomingOrder.Side == models.OrderSideBuy {
		return incomingOrder.Price.GreaterThanOrEqual(*restingOrder.Price)
	}
	return incomingOrder.Price.LessThanOrEqual(*restingOrder.Price)
}

// executeTrade creates a trade between two matching orders.
// Price selection rules:
// - Limit/Limit: use the resting order's price (price-time priority).
// - Market/Limit: use the limit order's price.
func (m *Matcher) executeTrade(incomingOrder, restingOrder *models.Order, executedAt time.Time) models.Trade {
	tradeQuantity := incomingOrder.RemainingQuantity
	if restingOrder.RemainingQuantity.LessThan(tradeQuantity) {
		tradeQuantity = restingOrder.RemainingQuantity
	}

	var tradePrice decimal.Decimal
	if incomingOrder.Type == models.OrderTypeMarket {
		tradePrice = *restingOrder.Price
	} else if restingOrder.Type == models.OrderTypeLimit {
		tradePrice = *restingOrder.Price
	} else {
		// fallback: use incoming order price if both have no limit (unlikely)
		tradePrice = *incomingOrder.Price
	}

	var buyOrderID, sellOrderID int64
	if incomingOrder.Side == models.OrderSideBuy {
		buyOrderID = incomingOrder.ID
		sellOrderID = restingOrder.ID
	} else {
		buyOrderID = restingOrder.ID
		sellOrderID = incomingOrder.ID
	}

	return models.Trade{
		Symbol:      incomingOrder.Symbol,
		BuyOrderID:  buyOrderID,
		SellOrderID: sellOrderID,
		Price:       tradePrice,
		Quantity:    tradeQuantity,
		ExecutedAt:  executedAt,
	}
}
