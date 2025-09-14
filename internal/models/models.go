package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// OrderSide represents the side of an order (buy or sell)
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType represents the type of an order (limit or market)
type OrderType string

const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

// OrderStatus represents the current status of an order
type OrderStatus string

const (
	OrderStatusOpen            OrderStatus = "open"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusCanceled        OrderStatus = "canceled"
)

// Order represents an order in the matching engine
type Order struct {
	ID                int64            `json:"id" db:"id"`
	ClientOrderID     *string          `json:"client_order_id,omitempty" db:"client_order_id"`
	Symbol            string           `json:"symbol" db:"symbol"`
	Side              OrderSide        `json:"side" db:"side"`
	Type              OrderType        `json:"type" db:"type"`
	Price             *decimal.Decimal `json:"price,omitempty" db:"price"`
	InitialQuantity   decimal.Decimal  `json:"initial_quantity" db:"initial_quantity"`
	RemainingQuantity decimal.Decimal  `json:"remaining_quantity" db:"remaining_quantity"`
	Status            OrderStatus      `json:"status" db:"status"`
	CreatedAt         time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at" db:"updated_at"`
}

// Trade represents a completed trade between two orders
type Trade struct {
	ID          int64           `json:"id" db:"id"`
	Symbol      string          `json:"symbol" db:"symbol"`
	BuyOrderID  int64           `json:"buy_order_id" db:"buy_order_id"`
	SellOrderID int64           `json:"sell_order_id" db:"sell_order_id"`
	Price       decimal.Decimal `json:"price" db:"price"`
	Quantity    decimal.Decimal `json:"quantity" db:"quantity"`
	ExecutedAt  time.Time       `json:"executed_at" db:"executed_at"`
}

// CreateOrderRequest represents the JSON payload for creating a new order
type CreateOrderRequest struct {
	ClientOrderID *string          `json:"client_order_id,omitempty"`
	Symbol        string           `json:"symbol" binding:"required"`
	Side          OrderSide        `json:"side" binding:"required"`
	Type          OrderType        `json:"type" binding:"required"`
	Price         *decimal.Decimal `json:"price,omitempty"`
	Quantity      decimal.Decimal  `json:"quantity" binding:"required"`
}

// CreateOrderResponse represents the response after creating an order
type CreateOrderResponse struct {
	OrderID int64   `json:"order_id"`
	Status  string  `json:"status"`
	Trades  []Trade `json:"trades,omitempty"`
	Message string  `json:"message"`
}

// OrderBookLevel represents a single price level in the order book
type OrderBookLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

// OrderBookResponse represents the aggregated order book response
type OrderBookResponse struct {
	Symbol string           `json:"symbol"`
	Bids   []OrderBookLevel `json:"bids"`
	Asks   []OrderBookLevel `json:"asks"`
}

// TradeResponse represents the response for trade queries
type TradeResponse struct {
	Trades []Trade `json:"trades"`
}
