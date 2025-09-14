package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"order-matching-engine/internal/db"
	"order-matching-engine/internal/engine"
	"order-matching-engine/internal/models"

	"github.com/joho/godotenv"
)

// Server wires together DB and matching engine and exposes HTTP handlers.
type Server struct {
	db     *sql.DB
	engine *engine.Engine
}

func main() {
	// Load environment variables if present (non-fatal).
	if err := godotenv.Load(); err != nil {
		log.Printf("[INFO] .env not loaded: %v", err)
	}

	log.Println("[INFO] Starting Order Matching Engine server...")

	// Connect to database.
	database, err := db.Connect()
	if err != nil {
		log.Fatalf("[ERROR] Failed to connect to database: %v", err)
	}
	defer func() {
		log.Println("[INFO] Closing database connection...")
		database.Close()
	}()
	log.Println("[INFO] Database connection established")

	// Initialize matching engine (prepares statements, etc).
	matchingEngine, err := engine.NewEngine(database)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create matching engine: %v", err)
	}
	defer func() {
		log.Println("[INFO] Closing matching engine...")
		matchingEngine.Close()
	}()
	log.Println("[INFO] Matching engine initialized")

	// Restore in-memory book state from DB.
	log.Println("[INFO] Loading open orders from database...")
	if err := matchingEngine.LoadOpenOrders(); err != nil {
		log.Fatalf("[ERROR] Failed to load open orders: %v", err)
	}

	srv := &Server{
		db:     database,
		engine: matchingEngine,
	}

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("/orders", srv.handleOrders)
	mux.HandleFunc("/orders/", srv.handleOrderByID)
	mux.HandleFunc("/trades", srv.handleTrades)
	mux.HandleFunc("/orderbook", srv.handleOrderBook)
	mux.HandleFunc("/health", srv.handleHealth)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Graceful shutdown setup.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("[INFO] Server starting on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[ERROR] Server failed: %v", err)
		}
	}()

	<-stop
	log.Println("[INFO] Shutdown signal received, initiating graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	} else {
		log.Println("[INFO] Server gracefully stopped")
	}
}

// handleOrders accepts POST /orders to create a new order.
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateCreateOrderRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[INFO] Processing order: symbol=%s, side=%s, type=%s, quantity=%s",
		req.Symbol, req.Side, req.Type, req.Quantity.String())

	order, trades, err := s.engine.PlaceOrder(&req)
	if err != nil {
		log.Printf("[ERROR] Failed to place order: symbol=%s, error=%v", req.Symbol, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] Order processed: id=%d, status=%s, trades=%d",
		order.ID, order.Status, len(trades))

	resp := models.CreateOrderResponse{
		OrderID: order.ID,
		Status:  string(order.Status),
		Trades:  trades,
		Message: "Order processed successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// handleOrderByID supports GET /orders/{id} and DELETE /orders/{id}.
func (s *Server) handleOrderByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/orders/")
	if path == "" {
		http.Error(w, "Order ID is required", http.StatusBadRequest)
		return
	}

	orderID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		order, err := s.engine.GetOrder(orderID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Order not found", http.StatusNotFound)
			} else {
				log.Printf("[ERROR] Failed to get order %d: %v", orderID, err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(order)
		return
	}

	// DELETE: cancel order
	log.Printf("[INFO] Canceling order: id=%d", orderID)
	order, err := s.engine.CancelOrder(orderID)
	if err != nil {
		log.Printf("[ERROR] Failed to cancel order: id=%d, error=%v", orderID, err)
		switch {
		case strings.Contains(err.Error(), "not found"):
			http.Error(w, "Order not found", http.StatusNotFound)
		case strings.Contains(err.Error(), "already filled"):
			http.Error(w, "Order already filled", http.StatusConflict)
		case strings.Contains(err.Error(), "already canceled"):
			http.Error(w, "Order already canceled", http.StatusConflict)
		case strings.Contains(err.Error(), "no remaining quantity"):
			http.Error(w, "Order has no remaining quantity", http.StatusConflict)
		case strings.Contains(err.Error(), "cannot be canceled"):
			http.Error(w, "Order cannot be canceled", http.StatusConflict)
		default:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	log.Printf("[INFO] Order canceled successfully: id=%d", orderID)
	resp := map[string]interface{}{
		"order_id": order.ID,
		"status":   string(order.Status),
		"message":  "Order canceled successfully",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleTrades returns recent trades for a symbol: GET /trades?symbol=...&limit=N
func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol parameter is required", http.StatusBadRequest)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
	}

	trades, err := s.engine.GetTrades(symbol, limit)
	if err != nil {
		log.Printf("[ERROR] Failed to get trades for symbol %s: %v", symbol, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := models.TradeResponse{Trades: trades}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleOrderBook returns aggregated top N levels: GET /orderbook?symbol=...&depth=N
func (s *Server) handleOrderBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol parameter is required", http.StatusBadRequest)
		return
	}

	depth := 10
	if depthStr := r.URL.Query().Get("depth"); depthStr != "" {
		var err error
		depth, err = strconv.Atoi(depthStr)
		if err != nil || depth < 1 || depth > 100 {
			http.Error(w, "Invalid depth parameter (must be 1-100)", http.StatusBadRequest)
			return
		}
	}

	bids, asks := s.engine.GetOrderBookWithQuantities(symbol, depth)
	response := models.OrderBookResponse{
		Symbol: symbol,
		Bids:   bids,
		Asks:   asks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleHealth is a simple health check that verifies DB connectivity.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.db.Ping(); err != nil {
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// validateCreateOrderRequest performs basic request validation for creating orders.
func validateCreateOrderRequest(req *models.CreateOrderRequest) error {
	if req.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if req.Side != models.OrderSideBuy && req.Side != models.OrderSideSell {
		return fmt.Errorf("side must be 'buy' or 'sell'")
	}
	if req.Type != models.OrderTypeLimit && req.Type != models.OrderTypeMarket {
		return fmt.Errorf("type must be 'limit' or 'market'")
	}
	if req.Quantity.IsZero() || req.Quantity.IsNegative() {
		return fmt.Errorf("quantity must be positive")
	}
	if req.Type == models.OrderTypeLimit {
		if req.Price == nil || req.Price.IsZero() || req.Price.IsNegative() {
			return fmt.Errorf("price is required for limit orders and must be positive")
		}
	}
	return nil
}
