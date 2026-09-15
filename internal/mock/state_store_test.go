package mock

import (
	"testing"
)

func TestStateStoreCRUD(t *testing.T) {
	store := NewStateStore()

	// 1. Create table
	createSQL := "CREATE TABLE users (id int, name text, email text);"
	rule, handled := store.MatchAndExecute(createSQL, nil)
	if !handled || rule.Tag != "CREATE TABLE" {
		t.Fatalf("expected CREATE TABLE handled, got rule: %+v, handled: %v", rule, handled)
	}

	// 2. Insert rows
	insertSQL := "INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@test.com'), (2, 'Bob', 'bob@test.com');"
	rule, handled = store.MatchAndExecute(insertSQL, nil)
	if !handled || rule.Tag != "INSERT 0 2" {
		t.Fatalf("expected INSERT 0 2, got: %+v", rule)
	}

	// 3. Select all
	selectSQL := "SELECT * FROM users;"
	rule, handled = store.MatchAndExecute(selectSQL, nil)
	if !handled || len(rule.Rows) != 2 {
		t.Fatalf("expected 2 rows, got: %+v", rule)
	}
	if rule.Rows[0][1] != "Alice" || rule.Rows[1][1] != "Bob" {
		t.Fatalf("unexpected rows: %+v", rule.Rows)
	}

	// 4. Select with WHERE and parameter binding
	selectWhereSQL := "SELECT name, email FROM users WHERE id = $1;"
	params := [][]byte{[]byte("2")}
	rule, handled = store.MatchAndExecute(selectWhereSQL, params)
	if !handled || len(rule.Rows) != 1 {
		t.Fatalf("expected 1 row matching id=2, got: %+v", rule)
	}
	if rule.Rows[0][0] != "Bob" {
		t.Fatalf("expected Bob, got: %s", rule.Rows[0][0])
	}

	// 5. Update
	updateSQL := "UPDATE users SET email = 'alice-new@test.com' WHERE name = 'Alice';"
	rule, handled = store.MatchAndExecute(updateSQL, nil)
	if !handled || rule.Tag != "UPDATE 1" {
		t.Fatalf("expected UPDATE 1, got: %+v", rule)
	}

	// Verify update
	rule, _ = store.MatchAndExecute("SELECT email FROM users WHERE name = 'Alice';", nil)
	if len(rule.Rows) != 1 || rule.Rows[0][0] != "alice-new@test.com" {
		t.Fatalf("update was not persisted: %+v", rule.Rows)
	}

	// 6. Delete
	deleteSQL := "DELETE FROM users WHERE id = 1;"
	rule, handled = store.MatchAndExecute(deleteSQL, nil)
	if !handled || rule.Tag != "DELETE 1" {
		t.Fatalf("expected DELETE 1, got: %+v", rule)
	}

	// Verify delete
	rule, _ = store.MatchAndExecute("SELECT COUNT(*) FROM users;", nil)
	if len(rule.Rows) != 1 || rule.Rows[0][0] != "1" {
		t.Fatalf("expected 1 remaining user, got: %+v", rule.Rows)
	}

	// 7. Non-existent table returns 42P01 error
	rule, handled = store.MatchAndExecute("SELECT * FROM non_existent_table;", nil)
	if !handled || rule.Error == nil || rule.Error.Code != "42P01" {
		t.Fatalf("expected 42P01 error for missing table, got: %+v", rule)
	}
}
