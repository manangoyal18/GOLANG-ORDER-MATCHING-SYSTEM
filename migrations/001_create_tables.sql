-- migrations/001_create_tables.sql
-- Note: Designed for MySQL-compatible servers (TiDB compatible).
-- Orders table
CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  client_order_id VARCHAR(255) NULL,
  symbol VARCHAR(64) NOT NULL,
  side ENUM('buy','sell') NOT NULL,
  type ENUM('limit','market') NOT NULL,
  price DECIMAL(30,10) NULL,
  initial_quantity DECIMAL(30,10) NOT NULL,
  remaining_quantity DECIMAL(30,10) NOT NULL,
  status ENUM('open','partially_filled','filled','canceled') NOT NULL DEFAULT 'open',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  metadata JSON NULL,
  -- Indexes
  INDEX idx_symbol_side_price (symbol, side, price),
  INDEX idx_status (status),
  INDEX idx_symbol_created (symbol, created_at),
  -- Optional unique constraint for client-specified IDs (kept non-enforced by default if NULL)
  UNIQUE KEY uq_client_order_id (client_order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- Trades (executions) table
CREATE TABLE IF NOT EXISTS trades (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  symbol VARCHAR(64) NOT NULL,
  buy_order_id BIGINT UNSIGNED NOT NULL,
  sell_order_id BIGINT UNSIGNED NOT NULL,
  price DECIMAL(30,10) NOT NULL,
  quantity DECIMAL(30,10) NOT NULL,
  executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  metadata JSON NULL,
  -- Foreign keys & indexes
  INDEX idx_symbol_executed_at (symbol, executed_at),
  INDEX idx_buy_order (buy_order_id),
  INDEX idx_sell_order (sell_order_id),
  CONSTRAINT fk_trades_buy_order FOREIGN KEY (buy_order_id)
    REFERENCES orders(id) ON DELETE RESTRICT ON UPDATE CASCADE,
  CONSTRAINT fk_trades_sell_order FOREIGN KEY (sell_order_id)
    REFERENCES orders(id) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;