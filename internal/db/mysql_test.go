package db

import (
	"os"
	"testing"
)

func TestConnect(t *testing.T) {
	// Test case 1: Missing DB_DSN environment variable
	originalDSN := os.Getenv("DB_DSN")
	os.Unsetenv("DB_DSN")

	_, err := Connect()
	if err == nil {
		t.Error("Expected error when DB_DSN is not set")
	}

	// Restore original DSN for other tests
	if originalDSN != "" {
		os.Setenv("DB_DSN", originalDSN)
	}

	// Test case 2: Invalid DSN format
	os.Setenv("DB_DSN", "invalid-dsn-format")

	_, err = Connect()
	if err == nil {
		t.Error("Expected error with invalid DSN format")
	}

	// Test case 3: Valid DSN format but potentially unreachable database
	// This test demonstrates the connection logic without requiring a live database
	testDSN := "testuser:testpass@tcp(localhost:3306)/testdb?parseTime=true"
	os.Setenv("DB_DSN", testDSN)

	db, err := Connect()
	// expect this to either succeed (if database is available) or fail with connection error
	if err != nil {
		t.Logf("Connection failed as expected (no test database): %v", err)
	} else {
		t.Log("Connection succeeded (test database is available)")
		db.Close()
	}

	if originalDSN != "" {
		os.Setenv("DB_DSN", originalDSN)
	} else {
		os.Unsetenv("DB_DSN")
	}
}

// Integration test that requires a real database connection
func TestConnectIntegration(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN environment variable not set, skipping integration test")
	}

	db, err := Connect()
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test basic query
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Fatalf("Failed to execute test query: %v", err)
	}

	if result != 1 {
		t.Errorf("Expected 1, got %d", result)
	}

	t.Log("Database connection test passed")
}

func TestConvertURIToDSN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "Traditional DSN passthrough",
			input:    "root:password@tcp(localhost:3306)/testdb?parseTime=true",
			expected: "root:password@tcp(localhost:3306)/testdb?parseTime=true",
			hasError: false,
		},
		{
			name:     "TiDB Cloud URI conversion",
			input:    "mysql://user.root:pass123@gateway01.region.prod.aws.tidbcloud.com:4000/testdb",
			expected: "user.root:pass123@tcp(gateway01.region.prod.aws.tidbcloud.com:4000)/testdb?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=true",
			hasError: false,
		},
		{
			name:     "URI without password",
			input:    "mysql://user@localhost:4000/testdb",
			expected: "user@tcp(localhost:4000)/testdb?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=true",
			hasError: false,
		},
		{
			name:     "URI without database defaults to test",
			input:    "mysql://user:pass@localhost:4000/",
			expected: "user:pass@tcp(localhost:4000)/test?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=true",
			hasError: false,
		},
		{
			name:     "Invalid scheme gets passed through as DSN",
			input:    "postgres://user:pass@localhost:5432/db",
			expected: "postgres://user:pass@localhost:5432/db",
			hasError: false,
		},
		{
			name:     "Malformed URI",
			input:    "mysql://invalid uri format",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertURIToDSN(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input %s, but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %s: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}
