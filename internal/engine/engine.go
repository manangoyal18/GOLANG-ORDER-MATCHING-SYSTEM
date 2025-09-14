package engine

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"order-matching-engine/internal/models"

	"github.com/shopspring/decimal"
)

// Engine is the main order matching engine.
// It holds DB connections, prepared statements, matcher and in-memory order books.
type Engine struct {
	db            *sql.DB
	matcher       *Matcher
	orderBooks    map[string]*OrderBook
	symbolMutexes map[string]*sync.Mutex
	globalMutex   sync.RWMutex

	// Prepared statements for common DB operations.
	insertOrderStmt *sql.Stmt
	insertTradeStmt *sql.Stmt
	updateOrderStmt *sql.Stmt
	selectOrderStmt *sql.Stmt
}

// NewEngine constructs an Engine and prepares SQL statements.
func NewEngine(db *sql.DB) (*Engine, error) {
	e := &Engine{
		db:            db,
		matcher:       NewMatcher(),
		orderBooks:    make(map[string]*OrderBook),
		symbolMutexes: make(map[string]*sync.Mutex),
	}

	if err := e.prepareStatements(); err != nil {
		return nil, fmt.Errorf("failed to prepare SQL statements: %w", err)
	}
	return e, nil
}

// prepareStatements prepares commonly used SQL statements.
func (e *Engine) prepareStatements() error {
	var err error

	e.insertOrderStmt, err = e.db.Prepare(`
		INSERT INTO orders (
			client_order_id, symbol, side, type, price, 
			initial_quantity, remaining_quantity, status, 
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert order statement: %w", err)
	}

	e.insertTradeStmt, err = e.db.Prepare(`
		INSERT INTO trades (
			symbol, buy_order_id, sell_order_id, price, quantity, executed_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert trade statement: %w", err)
	}

	e.updateOrderStmt, err = e.db.Prepare(`
		UPDATE orders 
		SET remaining_quantity = ?, status = ?, updated_at = ? 
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update order statement: %w", err)
	}

	e.selectOrderStmt, err = e.db.Prepare(`
		SELECT id, client_order_id, symbol, side, type, price, 
		       initial_quantity, remaining_quantity, status, created_at, updated_at
		FROM orders 
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare select order statement: %w", err)
	}

	return nil
}

// Close releases prepared statements held by the engine.
func (e *Engine) Close() error {
	stmts := []*sql.Stmt{
		e.insertOrderStmt,
		e.insertTradeStmt,
		e.updateOrderStmt,
		e.selectOrderStmt,
	}
	for _, s := range stmts {
		if s != nil {
			s.Close()
		}
	}
	return nil
}

// getSymbolMutex returns a per-symbol mutex, creating it if necessary.
// This provides coarse-grained serialization per trading symbol.
func (e *Engine) getSymbolMutex(symbol string) *sync.Mutex {
	e.globalMutex.RLock()
	mtx, ok := e.symbolMutexes[symbol]
	e.globalMutex.RUnlock()

	if !ok {
		e.globalMutex.Lock()
		if mtx, ok = e.symbolMutexes[symbol]; !ok {
			mtx = &sync.Mutex{}
			e.symbolMutexes[symbol] = mtx
		}
		e.globalMutex.Unlock()
	}
	return mtx
}

// getOrderBook returns the in-memory OrderBook for a symbol, creating it if necessary.
func (e *Engine) getOrderBook(symbol string) *OrderBook {
	e.globalMutex.RLock()
	ob, ok := e.orderBooks[symbol]
	e.globalMutex.RUnlock()

	if !ok {
		e.globalMutex.Lock()
		if ob, ok = e.orderBooks[symbol]; !ok {
			ob = NewOrderBook(symbol)
			e.orderBooks[symbol] = ob
		}
		e.globalMutex.Unlock()
	}
	return ob
}

// PlaceOrder processes a new order atomically:
// - acquires per-symbol lock
// - inserts the incoming order into DB within a transaction
// - runs in-memory matching
// - persists trades and order updates
// - commits the transaction
func (e *Engine) PlaceOrder(req *models.CreateOrderRequest) (*models.Order, []models.Trade, error) {
	// Per-symbol serialization to avoid cross-symbol interference.
	symbolMutex := e.getSymbolMutex(req.Symbol)
	symbolMutex.Lock()
	defer symbolMutex.Unlock()

	tx, err := e.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Protect against panic leaking a transaction.
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	now := time.Now()
	order := &models.Order{
		ClientOrderID:     req.ClientOrderID,
		Symbol:            req.Symbol,
		Side:              req.Side,
		Type:              req.Type,
		Price:             req.Price,
		InitialQuantity:   req.Quantity,
		RemainingQuantity: req.Quantity,
		Status:            models.OrderStatusOpen,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	var priceVal interface{}
	if order.Price != nil {
		priceVal = *order.Price
	}

	res, err := tx.Stmt(e.insertOrderStmt).Exec(
		order.ClientOrderID,
		order.Symbol,
		order.Side,
		order.Type,
		priceVal,
		order.InitialQuantity,
		order.RemainingQuantity,
		order.Status,
		order.CreatedAt,
		order.UpdatedAt,
	)
	if err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to insert order: %w", err)
	}

	orderID, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to get order ID: %w", err)
	}
	order.ID = orderID

	// In-memory matching against the book for the symbol.
	orderBook := e.getOrderBook(req.Symbol)
	matchResult := e.matcher.Match(order, orderBook)

	// Persist trades
	for _, trade := range matchResult.Trades {
		_, err = tx.Stmt(e.insertTradeStmt).Exec(
			trade.Symbol,
			trade.BuyOrderID,
			trade.SellOrderID,
			trade.Price,
			trade.Quantity,
			trade.ExecutedAt,
		)
		if err != nil {
			tx.Rollback()
			return nil, nil, fmt.Errorf("failed to insert trade: %w", err)
		}
	}

	// Persist order updates
	for _, updated := range matchResult.UpdatedOrders {
		_, err = tx.Stmt(e.updateOrderStmt).Exec(
			updated.RemainingQuantity,
			updated.Status,
			updated.UpdatedAt,
			updated.ID,
		)
		if err != nil {
			tx.Rollback()
			return nil, nil, fmt.Errorf("failed to update order %d: %w", updated.ID, err)
		}
	}

	// If incoming limit left, add to in-memory book and reflect final state.
	if matchResult.IncomingOrderLeft != nil {
		orderBook.AddOrder(matchResult.IncomingOrderLeft)
		*order = *matchResult.IncomingOrderLeft
	} else {
		// If fully filled/cancelled, update local order object from updated list.
		for _, u := range matchResult.UpdatedOrders {
			if u.ID == order.ID {
				*order = *u
				break
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return order, matchResult.Trades, nil
}

// GetOrder fetches an order by ID using the prepared select statement.
func (e *Engine) GetOrder(orderID int64) (*models.Order, error) {
	row := e.selectOrderStmt.QueryRow(orderID)

	var order models.Order
	var clientOrderID sql.NullString
	var price sql.NullString

	err := row.Scan(
		&order.ID,
		&clientOrderID,
		&order.Symbol,
		&order.Side,
		&order.Type,
		&price,
		&order.InitialQuantity,
		&order.RemainingQuantity,
		&order.Status,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("failed to scan order: %w", err)
	}

	if clientOrderID.Valid {
		order.ClientOrderID = &clientOrderID.String
	}
	if price.Valid {
		priceDecimal, err := decimal.NewFromString(price.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse price: %w", err)
		}
		order.Price = &priceDecimal
	}
	return &order, nil
}

// GetTrades returns recent trades for a symbol (limit 0 => no limit).
func (e *Engine) GetTrades(symbol string, limit int) ([]models.Trade, error) {
	query := `
		SELECT id, symbol, buy_order_id, sell_order_id, price, quantity, executed_at 
		FROM trades 
		WHERE symbol = ? 
		ORDER BY executed_at DESC, id DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := e.db.Query(query, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to query trades: %w", err)
	}
	defer rows.Close()

	var trades []models.Trade
	for rows.Next() {
		var t models.Trade
		if err := rows.Scan(
			&t.ID,
			&t.Symbol,
			&t.BuyOrderID,
			&t.SellOrderID,
			&t.Price,
			&t.Quantity,
			&t.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan trade: %w", err)
		}
		trades = append(trades, t)
	}
	return trades, nil
}

// GetOrderBook returns aggregated top levels for a symbol.
func (e *Engine) GetOrderBook(symbol string, depth int) (bids []PriceLevel, asks []PriceLevel) {
	ob := e.getOrderBook(symbol)
	return ob.GetTopLevels(depth)
}

// GetOrderBookWithQuantities returns aggregated levels with total quantities.
func (e *Engine) GetOrderBookWithQuantities(symbol string, depth int) ([]models.OrderBookLevel, []models.OrderBookLevel) {
	ob := e.getOrderBook(symbol)
	bidLevels, askLevels := ob.GetTopLevels(depth)

	bids := make([]models.OrderBookLevel, len(bidLevels))
	for i, lvl := range bidLevels {
		ob.mutex.RLock()
		pl := ob.Bids[lvl.Price.String()]
		total := decimal.Zero
		if pl != nil {
			total = pl.GetTotalQuantity()
		}
		ob.mutex.RUnlock()
		bids[i] = models.OrderBookLevel{Price: lvl.Price, Quantity: total}
	}

	asks := make([]models.OrderBookLevel, len(askLevels))
	for i, lvl := range askLevels {
		ob.mutex.RLock()
		pl := ob.Asks[lvl.Price.String()]
		total := decimal.Zero
		if pl != nil {
			total = pl.GetTotalQuantity()
		}
		ob.mutex.RUnlock()
		asks[i] = models.OrderBookLevel{Price: lvl.Price, Quantity: total}
	}

	return bids, asks
}

// CancelOrder cancels an open or partially filled order safely:
// - re-checks status inside a DB transaction to avoid races
// - updates DB, removes from in-memory book and commits
func (e *Engine) CancelOrder(orderID int64) (*models.Order, error) {
	order, err := e.GetOrder(orderID)
	if err != nil {
		return nil, err
	}

	if order.Status == models.OrderStatusFilled {
		return nil, fmt.Errorf("order already filled")
	}
	if order.Status == models.OrderStatusCanceled {
		return nil, fmt.Errorf("order already canceled")
	}
	if order.RemainingQuantity.IsZero() {
		return nil, fmt.Errorf("order has no remaining quantity")
	}

	// Per-symbol lock for atomicity.
	symMtx := e.getSymbolMutex(order.Symbol)
	symMtx.Lock()
	defer symMtx.Unlock()

	tx, err := e.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Re-check status inside transaction to avoid races.
	row := tx.Stmt(e.selectOrderStmt).QueryRow(orderID)
	var clientOrderID sql.NullString
	var price sql.NullString
	var temp models.Order
	var currentStatus string
	var currentRemainingQty decimal.Decimal

	if err := row.Scan(
		&temp.ID,
		&clientOrderID,
		&temp.Symbol,
		&temp.Side,
		&temp.Type,
		&price,
		&temp.InitialQuantity,
		&currentRemainingQty,
		&currentStatus,
		&temp.CreatedAt,
		&temp.UpdatedAt,
	); err != nil {
		tx.Rollback()
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("failed to re-check order status: %w", err)
	}

	if currentStatus == string(models.OrderStatusFilled) || currentStatus == string(models.OrderStatusCanceled) {
		tx.Rollback()
		return nil, fmt.Errorf("order cannot be canceled, current status: %s", currentStatus)
	}
	if currentRemainingQty.IsZero() {
		tx.Rollback()
		return nil, fmt.Errorf("order has no remaining quantity")
	}

	now := time.Now()
	if _, err := tx.Stmt(e.updateOrderStmt).Exec(decimal.Zero, models.OrderStatusCanceled, now, orderID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update order status: %w", err)
	}

	// Remove from in-memory book if present.
	ob := e.getOrderBook(order.Symbol)
	if order.Price != nil {
		ob.RemoveOrder(orderID, order.Side, order.Price)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	order.RemainingQuantity = decimal.Zero
	order.Status = models.OrderStatusCanceled
	order.UpdatedAt = now
	return order, nil
}

// LoadOpenOrders loads open and partially filled orders from DB and restores in-memory book.
// Call during startup to rebuild state.
func (e *Engine) LoadOpenOrders() error {
	query := `
		SELECT id, client_order_id, symbol, side, type, price, 
		       initial_quantity, remaining_quantity, status, created_at, updated_at
		FROM orders 
		WHERE status IN ('open', 'partially_filled') 
		ORDER BY created_at ASC, id ASC
	`

	rows, err := e.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query open orders: %w", err)
	}
	defer rows.Close()

	loaded := 0
	for rows.Next() {
		var order models.Order
		var clientOrderID sql.NullString
		var price sql.NullString

		if err := rows.Scan(
			&order.ID,
			&clientOrderID,
			&order.Symbol,
			&order.Side,
			&order.Type,
			&price,
			&order.InitialQuantity,
			&order.RemainingQuantity,
			&order.Status,
			&order.CreatedAt,
			&order.UpdatedAt,
		); err != nil {
			return fmt.Errorf("failed to scan order: %w", err)
		}

		if clientOrderID.Valid {
			order.ClientOrderID = &clientOrderID.String
		}
		if price.Valid {
			pd, err := decimal.NewFromString(price.String)
			if err != nil {
				return fmt.Errorf("failed to parse price for order %d: %w", order.ID, err)
			}
			order.Price = &pd
		}

		// Only limit orders are stored in the in-memory book.
		if order.Type == models.OrderTypeLimit && order.Price != nil {
			ob := e.getOrderBook(order.Symbol)
			ob.AddOrder(&order)
			loaded++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating orders: %w", err)
	}

	fmt.Printf("Loaded %d open orders into order books\n", loaded)
	return nil
}
