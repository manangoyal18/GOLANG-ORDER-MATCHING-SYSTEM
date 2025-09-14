-- Demo Data for Order Matching Engine
-- This script demonstrates typical trading scenarios and expected outcomes

-- Clean up any existing test data
DELETE FROM trades WHERE symbol IN ('BTCUSD', 'ETHUSDT');
DELETE FROM orders WHERE symbol IN ('BTCUSD', 'ETHUSDT');

-- Reset auto-increment counters
ALTER TABLE orders AUTO_INCREMENT = 1;
ALTER TABLE trades AUTO_INCREMENT = 1;

-- ============================================================================
-- SCENARIO 1: Basic Limit Order Matching
-- ============================================================================

-- Insert resting sell order at $50,000
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'sell-1', 'BTCUSD', 'sell', 'limit', '50000.00',
    '1.000000', '1.000000', 'open',
    '2024-01-01 10:00:00', '2024-01-01 10:00:00'
);

-- Insert matching buy order at $50,000 (should execute immediately)
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'buy-1', 'BTCUSD', 'buy', 'limit', '50000.00',
    '1.000000', '0.000000', 'filled',
    '2024-01-01 10:00:01', '2024-01-01 10:00:01'
);

-- Record the trade (normally done by the matching engine)
INSERT INTO trades (
    symbol, buy_order_id, sell_order_id, price, quantity, executed_at
) VALUES (
    'BTCUSD', 2, 1, '50000.00', '1.000000', '2024-01-01 10:00:01'
);

-- Update the sell order status to filled
UPDATE orders SET 
    remaining_quantity = '0.000000', 
    status = 'filled', 
    updated_at = '2024-01-01 10:00:01'
WHERE id = 1;

-- ============================================================================
-- SCENARIO 2: Partial Fill with Limit Order
-- ============================================================================

-- Insert large sell order at $50,100
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'sell-2', 'BTCUSD', 'sell', 'limit', '50100.00',
    '2.500000', '2.500000', 'open',
    '2024-01-01 10:01:00', '2024-01-01 10:01:00'
);

-- Insert smaller buy order that partially matches
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'buy-2', 'BTCUSD', 'buy', 'limit', '50100.00',
    '1.000000', '0.000000', 'filled',
    '2024-01-01 10:01:01', '2024-01-01 10:01:01'
);

-- Record the partial fill trade
INSERT INTO trades (
    symbol, buy_order_id, sell_order_id, price, quantity, executed_at
) VALUES (
    'BTCUSD', 4, 3, '50100.00', '1.000000', '2024-01-01 10:01:01'
);

-- Update sell order to partially filled status
UPDATE orders SET 
    remaining_quantity = '1.500000', 
    status = 'partially_filled', 
    updated_at = '2024-01-01 10:01:01'
WHERE id = 3;

-- ============================================================================
-- SCENARIO 3: Market Order Consuming Multiple Levels
-- ============================================================================

-- Set up multiple sell orders at different prices
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES 
    ('sell-3', 'BTCUSD', 'sell', 'limit', '50200.00', '0.500000', '0.000000', 'filled', '2024-01-01 10:02:00', '2024-01-01 10:02:02'),
    ('sell-4', 'BTCUSD', 'sell', 'limit', '50300.00', '0.800000', '0.000000', 'filled', '2024-01-01 10:02:00', '2024-01-01 10:02:02');

-- Large market buy order that consumes both levels
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'buy-market-1', 'BTCUSD', 'buy', 'market', NULL,
    '1.300000', '0.000000', 'filled',
    '2024-01-01 10:02:02', '2024-01-01 10:02:02'
);

-- Record trades from market order (multiple executions)
INSERT INTO trades (
    symbol, buy_order_id, sell_order_id, price, quantity, executed_at
) VALUES 
    ('BTCUSD', 7, 5, '50200.00', '0.500000', '2024-01-01 10:02:02'),
    ('BTCUSD', 7, 6, '50300.00', '0.800000', '2024-01-01 10:02:02');

-- ============================================================================
-- SCENARIO 4: FIFO Order at Same Price Level
-- ============================================================================

-- Insert two buy orders at same price (FIFO test)
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES 
    ('buy-fifo-1', 'BTCUSD', 'buy', 'limit', '49900.00', '0.700000', '0.000000', 'filled', '2024-01-01 10:03:00', '2024-01-01 10:03:02'),
    ('buy-fifo-2', 'BTCUSD', 'buy', 'limit', '49900.00', '0.300000', '0.300000', 'open', '2024-01-01 10:03:01', '2024-01-01 10:03:01');

-- Sell order that matches first buy order (FIFO)
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'sell-fifo-1', 'BTCUSD', 'sell', 'limit', '49900.00',
    '0.700000', '0.000000', 'filled',
    '2024-01-01 10:03:02', '2024-01-01 10:03:02'
);

-- Trade matches with first order (FIFO)
INSERT INTO trades (
    symbol, buy_order_id, sell_order_id, price, quantity, executed_at
) VALUES (
    'BTCUSD', 8, 10, '49900.00', '0.700000', '2024-01-01 10:03:02'
);

-- ============================================================================
-- SCENARIO 5: Order Cancellation
-- ============================================================================

-- Insert order that will be canceled
INSERT INTO orders (
    client_order_id, symbol, side, type, price, 
    initial_quantity, remaining_quantity, status, 
    created_at, updated_at
) VALUES (
    'buy-cancel-1', 'BTCUSD', 'buy', 'limit', '49800.00',
    '1.200000', '0.000000', 'canceled',
    '2024-01-01 10:04:00', '2024-01-01 10:04:30'
);

-- ============================================================================
-- EXPECTED RESULTS QUERIES
-- ============================================================================

-- Query 1: All orders with their current status
SELECT 
    id, client_order_id, symbol, side, type, 
    COALESCE(price, 'NULL') as price,
    initial_quantity, remaining_quantity, status,
    created_at, updated_at
FROM orders 
ORDER BY id;

-- Expected Results:
-- +----+---------------+--------+------+--------+----------+------------------+--------------------+-----------+---------------------+---------------------+
-- | id | client_order_id| symbol | side | type   | price    | initial_quantity | remaining_quantity | status    | created_at          | updated_at          |
-- +----+---------------+--------+------+--------+----------+------------------+--------------------+-----------+---------------------+---------------------+
-- |  1 | sell-1        | BTCUSD | sell | limit  | 50000.00 |      1.000000    |        0.000000    | filled    | 2024-01-01 10:00:00| 2024-01-01 10:00:01|
-- |  2 | buy-1         | BTCUSD | buy  | limit  | 50000.00 |      1.000000    |        0.000000    | filled    | 2024-01-01 10:00:01| 2024-01-01 10:00:01|
-- |  3 | sell-2        | BTCUSD | sell | limit  | 50100.00 |      2.500000    |        1.500000    | partially_filled | 2024-01-01 10:01:00| 2024-01-01 10:01:01|
-- |  4 | buy-2         | BTCUSD | buy  | limit  | 50100.00 |      1.000000    |        0.000000    | filled    | 2024-01-01 10:01:01| 2024-01-01 10:01:01|
-- |  5 | sell-3        | BTCUSD | sell | limit  | 50200.00 |      0.500000    |        0.000000    | filled    | 2024-01-01 10:02:00| 2024-01-01 10:02:02|
-- |  6 | sell-4        | BTCUSD | sell | limit  | 50300.00 |      0.800000    |        0.000000    | filled    | 2024-01-01 10:02:00| 2024-01-01 10:02:02|
-- |  7 | buy-market-1  | BTCUSD | buy  | market | NULL     |      1.300000    |        0.000000    | filled    | 2024-01-01 10:02:02| 2024-01-01 10:02:02|
-- |  8 | buy-fifo-1    | BTCUSD | buy  | limit  | 49900.00 |      0.700000    |        0.000000    | filled    | 2024-01-01 10:03:00| 2024-01-01 10:03:02|
-- |  9 | buy-fifo-2    | BTCUSD | buy  | limit  | 49900.00 |      0.300000    |        0.300000    | open      | 2024-01-01 10:03:01| 2024-01-01 10:03:01|
-- | 10 | sell-fifo-1   | BTCUSD | sell | limit  | 49900.00 |      0.700000    |        0.000000    | filled    | 2024-01-01 10:03:02| 2024-01-01 10:03:02|
-- | 11 | buy-cancel-1  | BTCUSD | buy  | limit  | 49800.00 |      1.200000    |        0.000000    | canceled  | 2024-01-01 10:04:00| 2024-01-01 10:04:30|
-- +----+---------------+--------+------+--------+----------+------------------+--------------------+-----------+---------------------+---------------------+

-- Query 2: All trades executed
SELECT 
    id, symbol, buy_order_id, sell_order_id, price, quantity, executed_at
FROM trades 
ORDER BY id;

-- Expected Results:
-- +----+--------+--------------+---------------+----------+----------+---------------------+
-- | id | symbol | buy_order_id | sell_order_id | price    | quantity | executed_at         |
-- +----+--------+--------------+---------------+----------+----------+---------------------+
-- |  1 | BTCUSD |            2 |             1 | 50000.00 | 1.000000 | 2024-01-01 10:00:01|
-- |  2 | BTCUSD |            4 |             3 | 50100.00 | 1.000000 | 2024-01-01 10:01:01|
-- |  3 | BTCUSD |            7 |             5 | 50200.00 | 0.500000 | 2024-01-01 10:02:02|
-- |  4 | BTCUSD |            7 |             6 | 50300.00 | 0.800000 | 2024-01-01 10:02:02|
-- |  5 | BTCUSD |            8 |            10 | 49900.00 | 0.700000 | 2024-01-01 10:03:02|
-- +----+--------+--------------+---------------+----------+----------+---------------------+

-- Query 3: Active orders that would be in the order book
SELECT 
    id, client_order_id, symbol, side, type, price,
    remaining_quantity, status, created_at
FROM orders 
WHERE status IN ('open', 'partially_filled')
ORDER BY 
    symbol, 
    side, 
    CASE WHEN side = 'buy' THEN -price ELSE price END,  -- Bids descending, asks ascending
    created_at;  -- FIFO within price level

-- Expected Results:
-- +----+---------------+--------+------+-------+----------+--------------------+-----------+---------------------+
-- | id | client_order_id| symbol | side | type  | price    | remaining_quantity | status    | created_at          |
-- +----+---------------+--------+------+-------+----------+--------------------+-----------+---------------------+
-- |  9 | buy-fifo-2    | BTCUSD | buy  | limit | 49900.00 |        0.300000    | open      | 2024-01-01 10:03:01|
-- |  3 | sell-2        | BTCUSD | sell | limit | 50100.00 |        1.500000    | partially_filled | 2024-01-01 10:01:00|
-- +----+---------------+--------+------+-------+----------+--------------------+-----------+---------------------+

-- Query 4: Summary statistics
SELECT 
    'Total Orders' as metric, COUNT(*) as value FROM orders
UNION ALL
SELECT 
    'Total Trades' as metric, COUNT(*) as value FROM trades
UNION ALL
SELECT 
    'Active Orders' as metric, COUNT(*) as value FROM orders WHERE status IN ('open', 'partially_filled')
UNION ALL
SELECT 
    'Total Volume Traded' as metric, CAST(SUM(quantity) as CHAR) as value FROM trades;

-- Expected Results:
-- +--------------------+---------+
-- | metric             | value   |
-- +--------------------+---------+
-- | Total Orders       |      11 |
-- | Total Trades       |       5 |
-- | Active Orders      |       2 |
-- | Total Volume Traded| 4.000000|
-- +--------------------+---------+