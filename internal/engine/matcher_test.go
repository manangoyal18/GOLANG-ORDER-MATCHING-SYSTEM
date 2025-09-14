package engine

import (
	"testing"
	"time"

	"order-matching-engine/internal/models"

	"github.com/shopspring/decimal"
)

// TestMatcher_LimitLimitFullMatch verifies a 1:1 limit/limit match results in one trade
// executed at the resting order's price and both orders marked filled.
func TestMatcher_LimitLimitFullMatch(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	sellPrice := decimal.NewFromInt(50000)
	sellOrder := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &sellPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder)

	buyPrice := decimal.NewFromInt(50000)
	incomingOrder := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeLimit,
		Price:             &buyPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Price.Cmp(sellPrice) != 0 {
		t.Errorf("Expected trade price %s, got %s", sellPrice.String(), trade.Price.String())
	}
	if trade.Quantity.Cmp(decimal.NewFromFloat(1.0)) != 0 {
		t.Errorf("Expected trade quantity 1.0, got %s", trade.Quantity.String())
	}
	if trade.BuyOrderID != 2 {
		t.Errorf("Expected buy order ID 2, got %d", trade.BuyOrderID)
	}
	if trade.SellOrderID != 1 {
		t.Errorf("Expected sell order ID 1, got %d", trade.SellOrderID)
	}

	if len(result.UpdatedOrders) != 2 {
		t.Fatalf("Expected 2 updated orders, got %d", len(result.UpdatedOrders))
	}

	for _, order := range result.UpdatedOrders {
		if order.Status != models.OrderStatusFilled {
			t.Errorf("Expected order %d to be filled, got status %s", order.ID, order.Status)
		}
		if !order.RemainingQuantity.IsZero() {
			t.Errorf("Expected order %d to have zero remaining quantity, got %s", order.ID, order.RemainingQuantity.String())
		}
	}

	if result.IncomingOrderLeft != nil {
		t.Error("Expected no incoming order left, but got one")
	}
}

// TestMatcher_LimitLimitPartialFill ensures a larger incoming limit buy partially fills
// a smaller resting sell, leaving the remainder as IncomingOrderLeft.
func TestMatcher_LimitLimitPartialFill(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	sellPrice := decimal.NewFromInt(50000)
	sellOrder := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &sellPrice,
		InitialQuantity:   decimal.NewFromFloat(0.5),
		RemainingQuantity: decimal.NewFromFloat(0.5),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder)

	buyPrice := decimal.NewFromInt(50000)
	incomingOrder := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeLimit,
		Price:             &buyPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Quantity.Cmp(decimal.NewFromFloat(0.5)) != 0 {
		t.Errorf("Expected trade quantity 0.5, got %s", trade.Quantity.String())
	}

	if len(result.UpdatedOrders) != 1 {
		t.Fatalf("Expected 1 updated order, got %d", len(result.UpdatedOrders))
	}

	sellUpdated := result.UpdatedOrders[0]
	if sellUpdated.Side != models.OrderSideSell {
		t.Errorf("Expected updated order to be sell order, got %s", sellUpdated.Side)
	}
	if sellUpdated.Status != models.OrderStatusFilled {
		t.Errorf("Expected sell order to be filled, got %s", sellUpdated.Status)
	}

	if result.IncomingOrderLeft == nil {
		t.Fatal("Expected incoming order to have leftover")
	}
	if result.IncomingOrderLeft.RemainingQuantity.Cmp(decimal.NewFromFloat(0.5)) != 0 {
		t.Errorf("Expected remaining quantity 0.5, got %s", result.IncomingOrderLeft.RemainingQuantity.String())
	}
	if result.IncomingOrderLeft.Status != models.OrderStatusPartiallyFilled {
		t.Errorf("Expected status partially_filled, got %s", result.IncomingOrderLeft.Status)
	}
}

// TestMatcher_MarketOrderConsumingMultipleLevels confirms a market buy walks the book
// across multiple ask levels producing trades at each resting price.
func TestMatcher_MarketOrderConsumingMultipleLevels(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	prices := []int64{50000, 50100, 50200}
	quantities := []float64{0.3, 0.4, 0.5}

	for i, price := range prices {
		sellPrice := decimal.NewFromInt(price)
		sellOrder := &models.Order{
			ID:                int64(i + 1),
			Symbol:            "BTCUSD",
			Side:              models.OrderSideSell,
			Type:              models.OrderTypeLimit,
			Price:             &sellPrice,
			InitialQuantity:   decimal.NewFromFloat(quantities[i]),
			RemainingQuantity: decimal.NewFromFloat(quantities[i]),
			Status:            models.OrderStatusOpen,
			CreatedAt:         time.Now().Add(-time.Duration(i+1) * time.Minute),
		}
		orderBook.AddOrder(sellOrder)
	}

	incomingOrder := &models.Order{
		ID:                4,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeMarket,
		Price:             nil,
		InitialQuantity:   decimal.NewFromFloat(1.2),
		RemainingQuantity: decimal.NewFromFloat(1.2),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 3 {
		t.Fatalf("Expected 3 trades, got %d", len(result.Trades))
	}

	expectedTrades := []struct {
		price    int64
		quantity float64
		sellID   int64
	}{
		{50000, 0.3, 1},
		{50100, 0.4, 2},
		{50200, 0.5, 3},
	}

	for i, expected := range expectedTrades {
		trade := result.Trades[i]
		if trade.Price.Cmp(decimal.NewFromInt(expected.price)) != 0 {
			t.Errorf("Trade %d: expected price %d, got %s", i, expected.price, trade.Price.String())
		}
		if trade.Quantity.Cmp(decimal.NewFromFloat(expected.quantity)) != 0 {
			t.Errorf("Trade %d: expected quantity %f, got %s", i, expected.quantity, trade.Quantity.String())
		}
		if trade.SellOrderID != expected.sellID {
			t.Errorf("Trade %d: expected sell order ID %d, got %d", i, expected.sellID, trade.SellOrderID)
		}
		if trade.BuyOrderID != 4 {
			t.Errorf("Trade %d: expected buy order ID 4, got %d", i, trade.BuyOrderID)
		}
	}

	if len(result.UpdatedOrders) != 4 {
		t.Fatalf("Expected 4 updated orders, got %d", len(result.UpdatedOrders))
	}

	for _, order := range result.UpdatedOrders {
		if order.Status != models.OrderStatusFilled {
			t.Errorf("Expected order %d to be filled, got status %s", order.ID, order.Status)
		}
	}

	if result.IncomingOrderLeft != nil {
		t.Error("Expected no incoming order left for fully filled market order")
	}
}

// TestMatcher_MarketOrderPartialCancel ensures market orders are canceled (not left on book)
// when they cannot be fully filled.
func TestMatcher_MarketOrderPartialCancel(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	sellPrice := decimal.NewFromInt(50000)
	sellOrder := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &sellPrice,
		InitialQuantity:   decimal.NewFromFloat(0.3),
		RemainingQuantity: decimal.NewFromFloat(0.3),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder)

	incomingOrder := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeMarket,
		Price:             nil,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Quantity.Cmp(decimal.NewFromFloat(0.3)) != 0 {
		t.Errorf("Expected trade quantity 0.3, got %s", trade.Quantity.String())
	}

	if len(result.UpdatedOrders) != 2 {
		t.Fatalf("Expected 2 updated orders, got %d", len(result.UpdatedOrders))
	}

	var marketUpdated *models.Order
	for _, order := range result.UpdatedOrders {
		if order.Type == models.OrderTypeMarket {
			marketUpdated = order
		}
	}

	if marketUpdated == nil {
		t.Fatal("Market order should be in updated orders")
	}
	if marketUpdated.Status != models.OrderStatusCanceled {
		t.Errorf("Expected market order to be canceled, got %s", marketUpdated.Status)
	}
	if !marketUpdated.RemainingQuantity.IsZero() {
		t.Errorf("Expected canceled market order to have zero remaining quantity, got %s", marketUpdated.RemainingQuantity.String())
	}

	if result.IncomingOrderLeft != nil {
		t.Error("Market order should not have leftover on book")
	}
}

// TestMatcher_FIFOSamePrice verifies FIFO ordering within the same price level.
func TestMatcher_FIFOSamePrice(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	sellPrice := decimal.NewFromInt(50000)

	sellOrder1 := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &sellPrice,
		InitialQuantity:   decimal.NewFromFloat(0.5),
		RemainingQuantity: decimal.NewFromFloat(0.5),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-2 * time.Minute),
	}
	orderBook.AddOrder(sellOrder1)

	sellOrder2 := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &sellPrice,
		InitialQuantity:   decimal.NewFromFloat(0.5),
		RemainingQuantity: decimal.NewFromFloat(0.5),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder2)

	buyPrice := decimal.NewFromInt(50000)
	incomingOrder := &models.Order{
		ID:                3,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeLimit,
		Price:             &buyPrice,
		InitialQuantity:   decimal.NewFromFloat(0.3),
		RemainingQuantity: decimal.NewFromFloat(0.3),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.SellOrderID != 1 {
		t.Errorf("Expected trade with sell order 1 (FIFO), got sell order %d", trade.SellOrderID)
	}
	if trade.Quantity.Cmp(decimal.NewFromFloat(0.3)) != 0 {
		t.Errorf("Expected trade quantity 0.3, got %s", trade.Quantity.String())
	}

	var firstOrderUpdated bool
	for _, order := range result.UpdatedOrders {
		if order.ID == 1 {
			firstOrderUpdated = true
			if order.Status != models.OrderStatusPartiallyFilled {
				t.Errorf("Expected first sell order to be partially filled, got %s", order.Status)
			}
			if order.RemainingQuantity.Cmp(decimal.NewFromFloat(0.2)) != 0 {
				t.Errorf("Expected remaining quantity 0.2, got %s", order.RemainingQuantity.String())
			}
		}
		if order.ID == 2 {
			t.Error("Second sell order should not be updated")
		}
	}

	if !firstOrderUpdated {
		t.Error("First sell order should be updated")
	}
}

// TestMatcher_PriceTimePriorityRules asserts that limit/limit matches execute at the resting order's price.
func TestMatcher_PriceTimePriorityRules(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	restingPrice := decimal.NewFromInt(50000)
	sellOrder := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &restingPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder)

	incomingPrice := decimal.NewFromInt(50100)
	incomingOrder := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeLimit,
		Price:             &incomingPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Price.Cmp(restingPrice) != 0 {
		t.Errorf("Expected trade price %s (resting order price), got %s", restingPrice.String(), trade.Price.String())
	}
}

// TestMatcher_MarketLimitPriceRule verifies market/limit matches use the limit (resting) order's price.
func TestMatcher_MarketLimitPriceRule(t *testing.T) {
	matcher := NewMatcher()
	orderBook := NewOrderBook("BTCUSD")

	limitPrice := decimal.NewFromInt(50000)
	sellOrder := &models.Order{
		ID:                1,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideSell,
		Type:              models.OrderTypeLimit,
		Price:             &limitPrice,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}
	orderBook.AddOrder(sellOrder)

	incomingOrder := &models.Order{
		ID:                2,
		Symbol:            "BTCUSD",
		Side:              models.OrderSideBuy,
		Type:              models.OrderTypeMarket,
		Price:             nil,
		InitialQuantity:   decimal.NewFromFloat(1.0),
		RemainingQuantity: decimal.NewFromFloat(1.0),
		Status:            models.OrderStatusOpen,
		CreatedAt:         time.Now(),
	}

	result := matcher.Match(incomingOrder, orderBook)

	if len(result.Trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]
	if trade.Price.Cmp(limitPrice) != 0 {
		t.Errorf("Expected trade price %s (limit order price), got %s", limitPrice.String(), trade.Price.String())
	}
}
