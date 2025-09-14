package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// convertURIToDSN converts a TiDB Cloud URI to MySQL DSN format
// Supports both mysql:// URI format and traditional DSN format
func convertURIToDSN(connectionString string) (string, error) {
	// If it doesn't start with mysql://, assume it's already a DSN
	if !strings.HasPrefix(connectionString, "mysql://") {
		return connectionString, nil
	}

	u, err := url.Parse(connectionString)
	if err != nil {
		return "", fmt.Errorf("failed to parse URI: %w", err)
	}

	if u.Scheme != "mysql" {
		return "", fmt.Errorf("unsupported scheme: %s (expected mysql)", u.Scheme)
	}

	if u.Host == "" {
		return "", fmt.Errorf("host is required")
	}

	var userInfo string
	if u.User != nil {
		username := u.User.Username()
		password, _ := u.User.Password()
		if password != "" {
			userInfo = username + ":" + password
		} else {
			userInfo = username
		}
	}

	// Get database name from path
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		database = "test" // Default database name
	}

	// Build DSN: user:password@tcp(host:port)/database
	dsn := fmt.Sprintf("%s@tcp(%s)/%s", userInfo, u.Host, database)

	// Add default query parameters for TiDB compatibility
	defaultParams := url.Values{
		"parseTime": []string{"true"},
		"charset":   []string{"utf8mb4"},
		"collation": []string{"utf8mb4_unicode_ci"},
	}

	// Merge with existing query parameters (existing params take precedence)
	existingParams := u.Query()
	for key, values := range defaultParams {
		if !existingParams.Has(key) {
			existingParams[key] = values
		}
	}

	// Add query parameters if any
	if len(existingParams) > 0 {
		dsn += "?" + existingParams.Encode()
	}

	return dsn, nil
}

// Connect establishes a connection to the MySQL/TiDB database
// using the DB_DSN environment variable
// Supports both traditional DSN format and TiDB Cloud URI format
func Connect() (*sql.DB, error) {
	connectionString := os.Getenv("DB_DSN")
	if connectionString == "" {
		return nil, fmt.Errorf("DB_DSN environment variable is required")
	}

	// Convert URI to DSN if needed
	dsn, err := convertURIToDSN(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to process connection string: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	return db, nil
}
