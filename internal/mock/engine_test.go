package mock

import (
	"testing"
)

func TestEngineExactAndNormalizedMatching(t *testing.T) {
	rules := []*Rule{
		{
			ID:      "users-list",
			Query:   "SELECT id, email, role FROM users",
			Columns: []string{"id", "email", "role"},
			Types:   []int32{23, 25, 25},
			Rows: [][]string{
				{"1", "admin@example.com", "admin"},
				{"2", "dev@example.com", "developer"},
			},
		},
	}

	eng := NewEngine(rules)

	// Test exact match
	m1 := eng.Match("SELECT id, email, role FROM users", nil)
	if m1.ID != "users-list" {
		t.Fatalf("expected rule users-list, got %q", m1.ID)
	}
	if len(m1.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(m1.Rows))
	}

	// Test normalized match (different whitespace, case, and trailing semicolon)
	m2 := eng.Match("  select   id,   email,  role   FROM   users ;  ", nil)
	if m2.ID != "users-list" {
		t.Fatalf("expected normalized rule users-list, got %q", m2.ID)
	}

	// Test builtin fallback for SELECT 1
	m3 := eng.Match("SELECT 1;", nil)
	if m3.ID != "builtin-select-1" {
		t.Fatalf("expected builtin-select-1, got %q", m3.ID)
	}
}

func TestEngineParameterizedMatching(t *testing.T) {
	rules := []*Rule{
		{
			ID:      "user-by-id-1",
			Query:   "SELECT * FROM users WHERE id = $1",
			Params:  []string{"1"},
			Columns: []string{"id", "name"},
			Rows:    [][]string{{"1", "Alice"}},
		},
		{
			ID:      "user-by-id-2",
			Query:   "SELECT * FROM users WHERE id = $1",
			Params:  []string{"2"},
			Columns: []string{"id", "name"},
			Rows:    [][]string{{"2", "Bob"}},
		},
	}

	eng := NewEngine(rules)

	m1 := eng.Match("SELECT * FROM users WHERE id = $1", [][]byte{[]byte("1")})
	if m1.ID != "user-by-id-1" {
		t.Fatalf("expected user-by-id-1, got %q", m1.ID)
	}

	m2 := eng.Match("SELECT * FROM users WHERE id = $1", [][]byte{[]byte("2")})
	if m2.ID != "user-by-id-2" {
		t.Fatalf("expected user-by-id-2, got %q", m2.ID)
	}
}

func TestEngineQueryLoggingAndReset(t *testing.T) {
	eng := NewEngine(nil)

	_ = eng.Match("SELECT 1", nil)
	_ = eng.Match("INSERT INTO logs VALUES ($1)", [][]byte{[]byte("test")})

	logs := eng.GetLogs()
	if len(logs) != 2 {
		t.Fatalf("expected 2 logged queries, got %d", len(logs))
	}
	if logs[0].Query != "SELECT 1" {
		t.Errorf("expected query SELECT 1, got %q", logs[0].Query)
	}
	if len(logs[1].Params) != 1 || logs[1].Params[0] != "test" {
		t.Errorf("expected param 'test', got %v", logs[1].Params)
	}

	eng.ResetLogs()
	if len(eng.GetLogs()) != 0 {
		t.Fatalf("expected 0 logs after reset")
	}
}
