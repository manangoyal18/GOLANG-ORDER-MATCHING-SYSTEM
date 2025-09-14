package engine

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"order-matching-engine/internal/db"
	"order-matching-engine/internal/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartupRecovery verifies that open/partially-filled orders are restored
// into the in-memory order books on engine startup.
func TestStartupRecovery(t *testing.T) {
	// Skip integration tests if no DSN is provided.
	if os.Getenv("DB_DSN") == "" {
		t.Skip("DB_DSN environment variable not set, skipping integration test")
	}

	database, err := db.Connect()
	require.NoError(t, err, "Failed to connect to test database")
	defer database.Close()

	cleanupTestData(t, database)

	now := time.Now()
	testOrders := []struct {
		id                int64
		symbol            string
		side              models.OrderSide
		orderType         models.OrderType
		price             decimal.Decimal
		initialQuantity   decimal.Decimal
		remainingQuantity decimal.Decimal
		status            models.OrderStatus
		createdAt         time.Time
	}{
		{1, "BTCUSD", models.OrderSideBuy, models.OrderTypeLimit, decimal.NewFromFloat(49000), decimal.NewFromFloat(1.5), decimal.NewFromFloat(1.5), models.OrderStatusOpen, now.Add(-5 * time.Minute)},
		{2, "BTCUSD", models.OrderSideBuy, models.OrderTypeLimit, decimal.NewFromFloat(49000), decimal.NewFromFloat(0.5), decimal.NewFromFloat(0.5), models.OrderStatusOpen, now.Add(-4 * time.Minute)},
		{3, "BTCUSD", models.OrderSideSell, models.OrderTypeLimit, decimal.NewFromFloat(51000), decimal.NewFromFloat(2.0), decimal.NewFromFloat(1.0), models.OrderStatusPartiallyFilled, now.Add(-3 * time.Minute)},
		{4, "ETHUSDT", models.OrderSideBuy, models.OrderTypeLimit, decimal.NewFromFloat(3000), decimal.NewFromFloat(3.0), decimal.NewFromFloat(3.0), models.OrderStatusOpen, now.Add(-2 * time.Minute)},
		{5, "ETHUSDT", models.OrderSideSell, models.OrderTypeLimit, decimal.NewFromFloat(3100), decimal.NewFromFloat(2.5), decimal.NewFromFloat(2.5), models.OrderStatusOpen, now.Add(-1 * time.Minute)},
	}

	insertStmt, err := database.Prepare(`
		INSERT INTO orders (
			id, client_order_id, symbol, side, type, price, 
			initial_quantity, remaining_quantity, status, 
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	require.NoError(t, err)
	defer insertStmt.Close()

	for _, o := range testOrders {
		_, err = insertStmt.Exec(
			o.id,
			nil,
			o.symbol,
			o.side,
			o.orderType,
			o.price,
			o.initialQuantity,
			o.remainingQuantity,
			o.status,
			o.createdAt,
			o.createdAt,
		)
		require.NoError(t, err, "Failed to insert test order %d", o.id)
	}

	eng, err := NewEngine(database)
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.LoadOpenOrders(), "Failed to load open orders")

	// BTCUSD checks
	btcOrderBook := eng.getOrderBook("BTCUSD")

	bestBid := btcOrderBook.GetBestBid()
	require.NotNil(t, bestBid, "Should have best bid order")
	assert.Equal(t, decimal.NewFromFloat(49000), *bestBid.Price)
	assert.Equal(t, int64(1), bestBid.ID)

	bestAsk := btcOrderBook.GetBestAsk()
	require.NotNil(t, bestAsk, "Should have best ask order")
	assert.Equal(t, decimal.NewFromFloat(51000), *bestAsk.Price)
	assert.Equal(t, int64(3), bestAsk.ID)
	assert.Equal(t, decimal.NewFromFloat(1.0), bestAsk.RemainingQuantity)

	// ETHUSDT checks
	ethOrderBook := eng.getOrderBook("ETHUSDT")

	ethBestBid := ethOrderBook.GetBestBid()
	require.NotNil(t, ethBestBid)
	assert.Equal(t, decimal.NewFromFloat(3000), *ethBestBid.Price)
	assert.Equal(t, int64(4), ethBestBid.ID)

	ethBestAsk := ethOrderBook.GetBestAsk()
	require.NotNil(t, ethBestAsk)
	assert.Equal(t, decimal.NewFromFloat(3100), *ethBestAsk.Price)
	assert.Equal(t, int64(5), ethBestAsk.ID)

	// FIFO within same price level for BTCUSD bids
	btcBids, _ := btcOrderBook.GetTopLevels(5)
	require.Len(t, btcBids, 1, "Should have exactly one bid price level")

	btcBidLevel := btcOrderBook.Bids["49000"]
	require.NotNil(t, btcBidLevel)
	require.Len(t, btcBidLevel.Orders, 2, "Should have 2 orders at 49000 price level")

	assert.Equal(t, int64(1), btcBidLevel.Orders[0].ID, "First order should be order 1")
	assert.Equal(t, int64(2), btcBidLevel.Orders[1].ID, "Second order should be order 2")
	assert.Equal(t, decimal.NewFromFloat(2.0), btcBidLevel.GetTotalQuantity(), "Total bid quantity should be 2.0")

	cleanupTestData(t, database)
}

// TestConcurrentOrderPlacement ensures concurrent placements for same symbol succeed
// and DB state remains consistent.
func TestConcurrentOrderPlacement(t *testing.T) {
	if os.Getenv("DB_DSN") == "" {
		t.Skip("DB_DSN environment variable not set, skipping integration test")
	}

	database, err := db.Connect()
	require.NoError(t, err)
	defer database.Close()

	cleanupTestData(t, database)

	eng, err := NewEngine(database)
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.LoadOpenOrders())

	const numGoroutines = 10
	const ordersPerGoroutine = 5

	results := make(chan error, numGoroutines*ordersPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for i := 0; i < ordersPerGoroutine; i++ {
				var side models.OrderSide
				var price decimal.Decimal

				if (goroutineID+i)%2 == 0 {
					side = models.OrderSideBuy
					price = decimal.NewFromFloat(49000 + float64(i*10))
				} else {
					side = models.OrderSideSell
					price = decimal.NewFromFloat(51000 + float64(i*10))
				}

				req := &models.CreateOrderRequest{
					Symbol:   "BTCUSD",
					Side:     side,
					Type:     models.OrderTypeLimit,
					Price:    &price,
					Quantity: decimal.NewFromFloat(0.1),
				}

				_, _, err := eng.PlaceOrder(req)
				results <- err
			}
		}(g)
	}

	for i := 0; i < numGoroutines*ordersPerGoroutine; i++ {
		err := <-results
		assert.NoError(t, err, "Order placement should not fail")
	}

	var orderCount int
	err = database.QueryRow("SELECT COUNT(*) FROM orders WHERE symbol = 'BTCUSD'").Scan(&orderCount)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*ordersPerGoroutine, orderCount, "Should have correct number of orders in database")

	rows, err := database.Query("SELECT id, status FROM orders WHERE symbol = 'BTCUSD' ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()

	validStatuses := map[string]bool{
		string(models.OrderStatusOpen):            true,
		string(models.OrderStatusPartiallyFilled): true,
		string(models.OrderStatusFilled):          true,
	}

	var checked int
	for rows.Next() {
		var id int64
		var status string
		err := rows.Scan(&id, &status)
		require.NoError(t, err)
		assert.True(t, validStatuses[status], "Order %d should have valid status, got: %s", id, status)
		checked++
	}

	assert.Equal(t, orderCount, checked, "Should have checked all orders")

	cleanupTestData(t, database)
}

// cleanupTestData removes test data for symbols used in integration tests.
func cleanupTestData(t *testing.T, database *sql.DB) {
	_, err := database.Exec("DELETE FROM trades WHERE symbol IN ('BTCUSD', 'ETHUSDT')")
	if err != nil {
		t.Logf("Warning: Failed to clean up test trades: %v", err)
	}

	_, err = database.Exec("DELETE FROM orders WHERE symbol IN ('BTCUSD', 'ETHUSDT')")
	if err != nil {
		t.Logf("Warning: Failed to clean up test orders: %v", err)
	}
}
