package engine

import (
	"sort"
	"sync"

	"order-matching-engine/internal/models"

	"github.com/shopspring/decimal"
)

// PriceLevel is a FIFO queue of orders at a specific price.
type PriceLevel struct {
	Price  decimal.Decimal
	Orders []*models.Order
}

// Add appends an order to the end of the price level (FIFO).
func (pl *PriceLevel) Add(order *models.Order) {
	pl.Orders = append(pl.Orders, order)
}

// Remove removes an order by ID and preserves FIFO order.
// Returns true if an order was removed.
func (pl *PriceLevel) Remove(orderID int64) bool {
	for i, order := range pl.Orders {
		if order.ID == orderID {
			pl.Orders = append(pl.Orders[:i], pl.Orders[i+1:]...)
			return true
		}
	}
	return false
}

// IsEmpty reports whether the price level has no orders.
func (pl *PriceLevel) IsEmpty() bool {
	return len(pl.Orders) == 0
}

// GetTotalQuantity sums remaining quantities at this price level.
func (pl *PriceLevel) GetTotalQuantity() decimal.Decimal {
	total := decimal.Zero
	for _, order := range pl.Orders {
		total = total.Add(order.RemainingQuantity)
	}
	return total
}

// OrderBook is the in-memory book for a single symbol.
// Concurrency: methods use the embedded mutex to be safe for concurrent use.
type OrderBook struct {
	Symbol string

	// Price lookup: string key is price.String()
	Bids map[string]*PriceLevel // bids indexed by price (descending)
	Asks map[string]*PriceLevel // asks indexed by price (ascending)

	// Cached sorted price slices for iteration (bidPrices: desc, askPrices: asc)
	bidPrices []decimal.Decimal
	askPrices []decimal.Decimal

	mutex sync.RWMutex
}

// NewOrderBook constructs an OrderBook for the given symbol.
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		Bids:   make(map[string]*PriceLevel),
		Asks:   make(map[string]*PriceLevel),
	}
}

// AddOrder inserts a limit order into the book. Market orders are not stored.
func (ob *OrderBook) AddOrder(order *models.Order) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// Do not add market orders to the book
	if order.Price == nil {
		return
	}
	priceKey := order.Price.String()

	if order.Side == models.OrderSideBuy {
		if ob.Bids[priceKey] == nil {
			ob.Bids[priceKey] = &PriceLevel{Price: *order.Price}
		}
		ob.Bids[priceKey].Add(order)
		ob.refreshBidPrices()
		return
	}

	if ob.Asks[priceKey] == nil {
		ob.Asks[priceKey] = &PriceLevel{Price: *order.Price}
	}
	ob.Asks[priceKey].Add(order)
	ob.refreshAskPrices()
}

// RemoveOrder deletes an order from the book by ID, side and price.
// Returns true if removal succeeded.
func (ob *OrderBook) RemoveOrder(orderID int64, side models.OrderSide, price *decimal.Decimal) bool {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	if price == nil {
		return false
	}
	priceKey := price.String()

	if side == models.OrderSideBuy {
		if pl := ob.Bids[priceKey]; pl != nil {
			if pl.Remove(orderID) {
				if pl.IsEmpty() {
					delete(ob.Bids, priceKey)
					ob.refreshBidPrices()
				}
				return true
			}
		}
		return false
	}

	if pl := ob.Asks[priceKey]; pl != nil {
		if pl.Remove(orderID) {
			if pl.IsEmpty() {
				delete(ob.Asks, priceKey)
				ob.refreshAskPrices()
			}
			return true
		}
	}
	return false
}

// GetBestBid returns the first (oldest) order at the highest bid price, or nil.
func (ob *OrderBook) GetBestBid() *models.Order {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	if len(ob.bidPrices) == 0 {
		return nil
	}
	bestPrice := ob.bidPrices[0]
	if pl := ob.Bids[bestPrice.String()]; pl != nil && len(pl.Orders) > 0 {
		return pl.Orders[0]
	}
	return nil
}

// GetBestAsk returns the first (oldest) order at the lowest ask price, or nil.
func (ob *OrderBook) GetBestAsk() *models.Order {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	if len(ob.askPrices) == 0 {
		return nil
	}
	bestPrice := ob.askPrices[0]
	if pl := ob.Asks[bestPrice.String()]; pl != nil && len(pl.Orders) > 0 {
		return pl.Orders[0]
	}
	return nil
}

// GetTopLevels returns up to depth aggregated price levels for each side.
// The returned PriceLevel structs contain only the Price (Orders == nil).
func (ob *OrderBook) GetTopLevels(depth int) (bids []PriceLevel, asks []PriceLevel) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	bidCount := depth
	if bidCount > len(ob.bidPrices) {
		bidCount = len(ob.bidPrices)
	}
	for i := 0; i < bidCount; i++ {
		price := ob.bidPrices[i]
		if pl := ob.Bids[price.String()]; pl != nil && !pl.IsEmpty() {
			bids = append(bids, PriceLevel{Price: price})
		}
	}

	askCount := depth
	if askCount > len(ob.askPrices) {
		askCount = len(ob.askPrices)
	}
	for i := 0; i < askCount; i++ {
		price := ob.askPrices[i]
		if pl := ob.Asks[price.String()]; pl != nil && !pl.IsEmpty() {
			asks = append(asks, PriceLevel{Price: price})
		}
	}
	return bids, asks
}

// refreshBidPrices rebuilds the cached bidPrices slice and sorts it descending.
func (ob *OrderBook) refreshBidPrices() {
	ob.bidPrices = make([]decimal.Decimal, 0, len(ob.Bids))
	for _, pl := range ob.Bids {
		if !pl.IsEmpty() {
			ob.bidPrices = append(ob.bidPrices, pl.Price)
		}
	}
	sort.Slice(ob.bidPrices, func(i, j int) bool {
		return ob.bidPrices[i].GreaterThan(ob.bidPrices[j]) // desc
	})
}

// refreshAskPrices rebuilds the cached askPrices slice and sorts it ascending.
func (ob *OrderBook) refreshAskPrices() {
	ob.askPrices = make([]decimal.Decimal, 0, len(ob.Asks))
	for _, pl := range ob.Asks {
		if !pl.IsEmpty() {
			ob.askPrices = append(ob.askPrices, pl.Price)
		}
	}
	sort.Slice(ob.askPrices, func(i, j int) bool {
		return ob.askPrices[i].LessThan(ob.askPrices[j]) // asc
	})
}

// GetOrderCount returns counts of bid and ask orders in the book.
func (ob *OrderBook) GetOrderCount() (bidCount, askCount int) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	for _, pl := range ob.Bids {
		bidCount += len(pl.Orders)
	}
	for _, pl := range ob.Asks {
		askCount += len(pl.Orders)
	}
	return bidCount, askCount
}
