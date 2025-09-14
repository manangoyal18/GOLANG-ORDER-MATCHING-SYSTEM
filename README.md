# Order Matching Engine

A complete Order Matching Engine in Go that provides a REST API for order management with TiDB/MySQL database support.

## Quick Start Guide

### Option 1: Docker Compose

```bash
# 1. Clone and navigate to the project
git clone <repository-url>
cd "GOLANG ORDER MATCHING SYSTEM"

# 2. Start MySQL and the service with one command
docker-compose up -d

# 3. Wait for services to be ready (30 seconds)
sleep 30

# 4. Test the service
curl http://localhost:8080/health

# 5. Run example commands (see examples below)
```

### Option 2: Manual Setup

```bash
# 1. Set up database (MySQL or TiDB)
# 2. Run migrations (see Database Setup section)
# 3. Set DB_DSN environment variable
# 4. Start the service: go run cmd/server/main.go
```

## Prerequisites

1. **Go 1.19+** installed
2. **TiDB** or **MySQL** database running

## Database Setup

### 1. Start TiDB or MySQL

For TiDB:

```bash
tiup playground
```

For MySQL:

```bash
mysql -u root -p
```

### 2. Run Migrations

The database schema is defined in `migrations/001_create_tables.sql`. This file contains the exact table definitions required.
**Apply the migration:**

```bash
# For local TiDB
mysql -h 127.0.0.1 -P 4000 -u root < migrations/001_create_tables.sql

# For local MySQL
mysql -h 127.0.0.1 -P 3306 -u root -p < migrations/001_create_tables.sql

# For TiDB Cloud
mysql --host=gateway01.ap-southeast-1.prod.aws.tidbcloud.com --port=4000 -u <user> -p -D test < migrations/001_create_tables.sql
```

### 3. Configure Environment Variables

The application loads configuration from a `.env` file. Copy and customize the provided sample:

```bash
# Copy the sample .env file
cp .env .env.local

# Edit the .env file with your database credentials
```

#### Database Connection Options:

The `DB_DSN` environment variable supports two formats:

**Option A: Traditional DSN Format (Local Development)**

```bash
# Local TiDB
DB_DSN="root:@tcp(127.0.0.1:4000)/orders_db?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

# Local MySQL
DB_DSN="root:password@tcp(127.0.0.1:3306)/orders_db?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"
```

**Option B: TiDB Cloud URI Format (Cloud Deployment)**

```bash
# TiDB Cloud (automatically converted to DSN format)
DB_DSN="mysql://username.root:password@gateway01.region.prod.aws.tidbcloud.com:4000/database_name"
```

## Step-by-Step Manual Setup

### 1. Database Setup

**Option A: Using Docker MySQL (Quick)**

```bash
# Start MySQL with Docker
docker run --name orderbook-mysql -d \
  -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=password123 \
  -e MYSQL_DATABASE=orders_db \
  -e MYSQL_USER=orderbook \
  -e MYSQL_PASSWORD=orderbook123 \
  mysql:8.0

sleep 30
```

**Option B: Local MySQL**

```bash
sudo systemctl start mysql  # Linux
brew services start mysql   # macOS

# Create database
mysql -u root -p -e "CREATE DATABASE orders_db;"
```

**Option C: TiDB Local**

```bash
# Install and start TiDB playground
curl --proto '=https' --tlsv1.2 -sSf https://tiup.io/install.sh | sh
tiup playground  # Runs on port 4000
```

### 2. Run Database Migrations

Apply the migration from `migrations/001_create_tables.sql` to create the required `orders` and `trades` tables:

```bash
# For Docker MySQL
mysql -h 127.0.0.1 -P 3306 -u orderbook -porderbook123 orders_db < migrations/001_create_tables.sql

# For local MySQL
mysql -h 127.0.0.1 -P 3306 -u root -p orders_db < migrations/001_create_tables.sql

# For local TiDB
mysql -h 127.0.0.1 -P 4000 -u root orders_db < migrations/001_create_tables.sql

# For TiDB Cloud (replace with your connection details)
mysql --host=gateway01.ap-southeast-1.prod.aws.tidbcloud.com --port=4000 -u <user> -p -D <database> < migrations/001_create_tables.sql
```

### 3. Configure Database Connection

**Create .env file:**

```bash
# Copy sample .env and edit
cp .env.example .env

# Edit .env file with your database credentials
```

**Example .env configurations:**

```bash
# Docker MySQL
DB_DSN="orderbook:orderbook123@tcp(127.0.0.1:3306)/orders_db?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

# Local MySQL
DB_DSN="root:your_password@tcp(127.0.0.1:3306)/orders_db?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

# Local TiDB
DB_DSN="root:@tcp(127.0.0.1:4000)/orders_db?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"

# TiDB Cloud
DB_DSN="mysql://username.root:password@gateway01.region.prod.aws.tidbcloud.com:4000/database_name"
```

### 4. Install Dependencies and Start Server

```bash
go mod tidy
go run cmd/server/main.go

# Server will start on http://localhost:8080
# log message: [INFO] Server starting on :8080
```

### 5. Verify Installation

```bash
# Test health endpoint
curl http://localhost:8080/health
# Expected: {"status":"healthy"}

# Test with invalid request (should return 400)
curl -X POST http://localhost:8080/orders -H "Content-Type: application/json" -d '{}'
# Expected: HTTP 400 with error message
```

## API Endpoints

### POST /orders

Create and place a new order. The order will be matched immediately against existing orders in the book.

**Request Body:**

```json
{
  "client_order_id": "client-123", // optional
  "symbol": "BTCUSD",
  "side": "buy", // "buy" or "sell"
  "type": "limit", // "limit" or "market"
  "price": "50000.50", // required for limit orders
  "quantity": "1.5"
}
```

**Response (201 Created):**

```json
{
  "order_id": 1,
  "status": "filled", // "open", "partially_filled", "filled", or "canceled"
  "trades": [
    // immediate trades executed (if any)
    {
      "id": 0, // auto-generated in DB
      "symbol": "BTCUSD",
      "buy_order_id": 1,
      "sell_order_id": 2,
      "price": "50000.00",
      "quantity": "1.5",
      "executed_at": "2023-01-01T12:00:00Z"
    }
  ],
  "message": "Order processed successfully"
}
```

### GET /orders/{id}

Retrieve details of a specific order by ID.

**Response (200 OK):**

```json
{
  "id": 1,
  "client_order_id": "client-123",
  "symbol": "BTCUSD",
  "side": "buy",
  "type": "limit",
  "price": "50000.50",
  "initial_quantity": "1.5",
  "remaining_quantity": "0.0", // 0 if fully filled
  "status": "filled",
  "created_at": "2023-01-01T12:00:00Z",
  "updated_at": "2023-01-01T12:00:01Z"
}
```

### DELETE /orders/{id}

Cancel a pending order (open or partially_filled status only).

**Response (200 OK):**

```json
{
  "order_id": 1,
  "status": "canceled",
  "message": "Order canceled successfully"
}
```

**Error Responses:**

- `404 Not Found`: Order not found
- `409 Conflict`: Order already filled, already canceled, or has no remaining quantity

### GET /trades?symbol=BTCUSD&limit=100

List recent trades for a symbol.

**Response (200 OK):**

```json
{
  "trades": [
    {
      "id": 1,
      "symbol": "BTCUSD",
      "buy_order_id": 1,
      "sell_order_id": 2,
      "price": "50000.00",
      "quantity": "1.5",
      "executed_at": "2023-01-01T12:00:00Z"
    }
  ]
}
```

### GET /orderbook?symbol=BTCUSD&depth=10

Get current order book state with aggregated price levels.

**Response (200 OK):**

```json
{
  "symbol": "BTCUSD",
  "bids": [
    // sorted by price descending (best first)
    {
      "price": "49950.00",
      "quantity": "2.5" // total quantity at this price level
    },
    {
      "price": "49900.00",
      "quantity": "1.0"
    }
  ],
  "asks": [
    // sorted by price ascending (best first)
    {
      "price": "50050.00",
      "quantity": "1.8"
    },
    {
      "price": "50100.00",
      "quantity": "3.2"
    }
  ]
}
```

### GET /health

Check server and database health.

**Response (200 OK):**

```json
{
  "status": "healthy"
}
```

## Example Usage & Order Matching Behavior

### Basic Order Placement

```bash
# Create a limit sell order at $50,000 (goes into order book)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "1.0"
  }'

# Response: {"order_id": 1, "status": "open", "trades": [], "message": "Order processed successfully"}

# Create a matching limit buy order (executes immediately)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "limit",
    "price": "50000.00",
    "quantity": "1.0"
  }'

# Response: {"order_id": 2, "status": "filled", "trades": [{"symbol": "BTCUSD", "buy_order_id": 2, "sell_order_id": 1, "price": "50000.00", "quantity": "1.0", "executed_at": "..."}], "message": "Order processed successfully"}
```

### Price-Time Priority Matching

```bash
# Set up multiple sell orders at same price
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "0.5"
  }'

# Second sell order at same price (will be second in FIFO queue)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "0.3"
  }'

# Buy order will match first sell order first (FIFO within price level)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "limit",
    "price": "50000.00",
    "quantity": "0.2"
  }'
```

### Market Orders Consuming Multiple Levels

```bash
# Create sell orders at different prices
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "0.5"
  }'

curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50100.00",
    "quantity": "0.7"
  }'

# Large market buy order consumes both levels (best prices first)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "market",
    "quantity": "1.0"
  }'

# Response will show 2 trades: 0.5 @ $50,000 and 0.5 @ $50,100
```

### Partial Fills and Leftovers

```bash
# Small sell order
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "0.3"
  }'

# Larger buy limit order (gets partially filled, leftover stays on book)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "limit",
    "price": "50000.00",
    "quantity": "1.0"
  }'

# Response: {"status": "partially_filled", "trades": [{"quantity": "0.3", ...}], ...}
# Remaining 0.7 quantity stays on order book

# Market order with insufficient liquidity (leftover gets canceled)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "market",
    "quantity": "5.0"
  }'

# If only 1.0 available, trades 1.0 and cancels remaining 4.0
```

### Querying Data

```bash
# Get specific order details
curl http://localhost:8080/orders/1

# Cancel a pending order
curl -X DELETE http://localhost:8080/orders/1

# Get recent trades
curl "http://localhost:8080/trades?symbol=BTCUSD&limit=50"

# Get current order book (top 5 levels each side)
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=5"

# Check server health
curl http://localhost:8080/health
```

## Complete Testing Workflow

Here's a complete sequence of curl commands that demonstrate all key features:

```bash
# 1. Verify service is running
curl http://localhost:8080/health

# 2. Check empty order book
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=5"

# 3. Place a limit sell order (goes into order book)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "sell",
    "type": "limit",
    "price": "50000.00",
    "quantity": "1.5"
  }'
# Response: order_id=1, status="open"

# 4. Check order book (should show the sell order)
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=5"

# 5. Place matching buy order (should execute immediately)
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "limit",
    "price": "50000.00",
    "quantity": "1.0"
  }'
# Response: order_id=2, status="filled", trades=[{...}]

# 6. Check order details (should show partial fill for order 1)
curl http://localhost:8080/orders/1
# Response: remaining_quantity="0.5", status="partially_filled"

curl http://localhost:8080/orders/2
# Response: remaining_quantity="0.0", status="filled"

# 7. Check trades
curl "http://localhost:8080/trades?symbol=BTCUSD&limit=10"
# Should show 1 trade: buy_order_id=2, sell_order_id=1, quantity=1.0

# 8. Check order book (should show remaining 0.5 from order 1)
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=5"

# 9. Place market order to consume remaining liquidity
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "market",
    "quantity": "0.5"
  }'
# Response: order_id=3, status="filled", trades=[{...}]

# 10. Verify order book is now empty
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=5"

# 11. Place an order to cancel
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSD",
    "side": "buy",
    "type": "limit",
    "price": "49000.00",
    "quantity": "2.0"
  }'
# Response: order_id=4, status="open"

# 12. Cancel the order
curl -X DELETE http://localhost:8080/orders/4
# Response: order_id=4, status="canceled"

# 13. Try to cancel again (should return 409 Conflict)
curl -X DELETE http://localhost:8080/orders/4
# Response: HTTP 409 "Order already canceled"

# 14. Final verification - check all trades
curl "http://localhost:8080/trades?symbol=BTCUSD&limit=10"
# Should show 2 trades total
```

## Demo Data Script

For comprehensive testing, you can run the demo data script:

```bash
# Run the demo script to populate database with test scenarios
mysql -h 127.0.0.1 -P 3306 -u orderbook -porderbook123 orders_db < examples/demo_data.sql

# Restart the service to load the test data
# The LoadOpenOrders() function will reconstruct in-memory order books

# Check the results
curl "http://localhost:8080/orderbook?symbol=BTCUSD&depth=10"
curl "http://localhost:8080/trades?symbol=BTCUSD&limit=20"
```

## Database Schema

The database schema is defined in `migrations/001_create_tables.sql` and is designed to be compatible with both MySQL and TiDB Cloud. This migration file is the **authoritative schema definition** and must be applied to create the required tables.

**Key Schema Features:**

- `DECIMAL(30,10)` for all price and quantity fields to prevent floating-point errors
- `JSON` columns for extensible metadata storage
- Proper foreign key constraints with `ON DELETE RESTRICT` and `ON UPDATE CASCADE`
- Optimized indexes for efficient querying (price-time priority, trade lookups)
- `BIGINT UNSIGNED` for all ID fields
- TiDB/MySQL compatible syntax with proper charset and collation

### Orders Table

- `id`: Unique order identifier
- `client_order_id`: Optional client-provided identifier
- `symbol`: Trading pair (e.g., "BTCUSD")
- `side`: "buy" or "sell"
- `type`: "limit" or "market"
- `price`: Order price (required for limit orders)
- `initial_quantity`: Original order quantity
- `remaining_quantity`: Unfilled quantity
- `status`: "open", "partially_filled", "filled", or "canceled"
- `created_at`/`updated_at`: Timestamps

### Trades Table

- `id`: Unique trade identifier
- `symbol`: Trading pair
- `buy_order_id`/`sell_order_id`: References to matched orders
- `price`: Execution price
- `quantity`: Executed quantity
- `executed_at`: Execution timestamp

## Testing

### Unit Tests:

```bash
go test ./internal/engine
```

### Integration Tests:

```bash
# Run all tests
go test ./...

# Test with verbose output
go test -v ./...
```

#### Running Integration Tests

These tests require a running database and the `DB_DSN` environment variable to be set:

```bash
# Set database connection and run integration tests
export DB_DSN="your_database_connection_string"
go test -v ./internal/engine -run "TestStartupRecovery|TestConcurrentOrderPlacement"

# Or run all engine tests (unit + integration)
go test -v ./internal/engine
```

## Design Decisions and Assumptions

### Core Matching Algorithm

**Price Determination Rules:**

- **Limit vs Limit**: Trade executes at the **resting order's price** (price-time priority principle)
- **Market vs Limit**: Trade executes at the **limit order's price**
- **Rationale**: The resting order was placed first and deserves price priority; market orders accept whatever price is available

**Decimal Precision:**

- All financial calculations use `github.com/shopspring/decimal` package
- Prevents floating-point precision errors that could cause monetary discrepancies
- All prices and quantities stored as DECIMAL(20,6) in database for consistency

**Order Matching Priority:**

1. **Price priority**: Best prices matched first (highest bid, lowest ask)
2. **Time priority**: Within same price level, orders matched in FIFO order
3. **Implementation**: Sorted slices with cached price levels for O(log n) performance

### Transaction Atomicity

**Single Transaction Principle:**

- Every order placement operation happens within a single database transaction
- Order insertion → matching → trade recording → order updates are atomic
- If any step fails, entire operation is rolled back
- Prevents partial state corruption during system failures

**Recovery and Consistency:**

- `LoadOpenOrders()` rebuilds in-memory state from database on startup
- Orders loaded in chronological order to maintain FIFO semantics
- Only open and partially_filled orders are loaded into order books

### Concurrency Model

**Per-Symbol Locking:**

- Each trading symbol has its own mutex to minimize contention
- Orders for different symbols can be processed concurrently
- Within a symbol, operations are strictly sequential to ensure consistency

**Single-Process Assumption:**

- Engine designed for single-process deployment (not distributed)
- In-memory order books are authoritative for matching decisions
- Database serves as persistent storage and recovery mechanism
- For multi-instance deployment, would require distributed locking or message queues

### Partial Fill Semantics

**Limit Orders:**

- Unfilled portions remain on order book with `partially_filled` status
- Can be matched against future incoming orders
- Remain active until fully filled or explicitly canceled

**Market Orders:**

- Unfilled portions are automatically canceled (never stay on book)
- Ensures market orders don't create stale liquidity at undefined prices
- Status changes from `open` to `filled` or `canceled` only

### Error Handling Strategy

**Input Validation:**

- Quantity must be positive (> 0)
- Price required and positive for limit orders
- Symbol and side validation with clear error messages
- HTTP status codes: 400 for validation, 404 for not found, 409 for conflicts

**Database Error Recovery:**

- Connection failures logged with structured logging
- Graceful shutdown ensures in-flight transactions complete
- Prepared statements improve performance and prevent SQL injection
