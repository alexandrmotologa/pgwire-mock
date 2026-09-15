package mock

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// InMemTable holds columns, data types, and stored row data
type InMemTable struct {
	Name    string
	Columns []string
	Types   []int32
	Rows    [][]string
}

// StateStore provides an in-memory SQL database for stateful CRUD mock operations
type StateStore struct {
	mu     sync.RWMutex
	tables map[string]*InMemTable
}

// NewStateStore creates an empty in-memory state store
func NewStateStore() *StateStore {
	return &StateStore{
		tables: make(map[string]*InMemTable),
	}
}

// Reset clears all in-memory tables and records
func (s *StateStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tables = make(map[string]*InMemTable)
}

var (
	createTableRegex = regexp.MustCompile(`(?i)^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_\.]+)\s*\((.+)\)`)
	dropTableRegex   = regexp.MustCompile(`(?i)^DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?([a-zA-Z0-9_\.]+)`)
	truncateRegex    = regexp.MustCompile(`(?i)^TRUNCATE\s+(?:TABLE\s+)?([a-zA-Z0-9_\.]+)`)
	insertRegex      = regexp.MustCompile(`(?i)^INSERT\s+INTO\s+([a-zA-Z0-9_\.]+)(?:\s*\(([^)]+)\))?\s+VALUES\s*(.+)`)
	selectRegex      = regexp.MustCompile(`(?i)^SELECT\s+(.+?)\s+FROM\s+([a-zA-Z0-9_\.]+)(?:\s+WHERE\s+(.+))?`)
	updateRegex      = regexp.MustCompile(`(?i)^UPDATE\s+([a-zA-Z0-9_\.]+)\s+SET\s+(.+?)(?:\s+WHERE\s+(.+))?$`)
	deleteRegex      = regexp.MustCompile(`(?i)^DELETE\s+FROM\s+([a-zA-Z0-9_\.]+)(?:\s+WHERE\s+(.+))?`)
)

// MatchAndExecute attempts to execute the query against the in-memory state store.
// Returns (rule, handled). If handled is false, query is not a supported stateful operation.
func (s *StateStore) MatchAndExecute(query string, params [][]byte) (*Rule, bool) {
	trimmed := strings.TrimRight(strings.TrimSpace(query), ";")

	// 1. CREATE TABLE
	if matches := createTableRegex.FindStringSubmatch(trimmed); len(matches) > 2 {
		tableName := sanitizeName(matches[1])
		colDefs := strings.Split(matches[2], ",")
		columns := make([]string, 0, len(colDefs))
		types := make([]int32, 0, len(colDefs))

		for _, colDef := range colDefs {
			parts := strings.Fields(strings.TrimSpace(colDef))
			if len(parts) > 0 {
				colName := sanitizeName(parts[0])
				// Ignore table constraints like PRIMARY KEY (id), FOREIGN KEY...
				if strings.EqualFold(colName, "PRIMARY") || strings.EqualFold(colName, "FOREIGN") ||
					strings.EqualFold(colName, "CONSTRAINT") || strings.EqualFold(colName, "UNIQUE") {
					continue
				}
				columns = append(columns, colName)
				types = append(types, 25) // default text OID
			}
		}

		s.mu.Lock()
		s.tables[tableName] = &InMemTable{
			Name:    tableName,
			Columns: columns,
			Types:   types,
			Rows:    make([][]string, 0),
		}
		s.mu.Unlock()

		return &Rule{
			ID:    "stateful-create-table",
			Query: query,
			Tag:   "CREATE TABLE",
		}, true
	}

	// 2. DROP TABLE
	if matches := dropTableRegex.FindStringSubmatch(trimmed); len(matches) > 1 {
		tableName := sanitizeName(matches[1])
		s.mu.Lock()
		delete(s.tables, tableName)
		s.mu.Unlock()

		return &Rule{
			ID:    "stateful-drop-table",
			Query: query,
			Tag:   "DROP TABLE",
		}, true
	}

	// 3. TRUNCATE TABLE
	if matches := truncateRegex.FindStringSubmatch(trimmed); len(matches) > 1 {
		tableName := sanitizeName(matches[1])
		s.mu.Lock()
		if tbl, exists := s.tables[tableName]; exists {
			tbl.Rows = make([][]string, 0)
		}
		s.mu.Unlock()

		return &Rule{
			ID:    "stateful-truncate-table",
			Query: query,
			Tag:   "TRUNCATE TABLE",
		}, true
	}

	// 4. INSERT INTO
	if matches := insertRegex.FindStringSubmatch(trimmed); len(matches) > 3 {
		tableName := sanitizeName(matches[1])
		colListStr := matches[2]
		valuesRest := matches[3]

		// Check for RETURNING clause
		returningClause := ""
		upperRest := strings.ToUpper(valuesRest)
		if idx := strings.Index(upperRest, " RETURNING "); idx != -1 {
			returningClause = strings.TrimSpace(valuesRest[idx+len(" RETURNING "):])
			valuesRest = strings.TrimSpace(valuesRest[:idx])
		}

		// Parse values tuples: (v1, v2), (v3, v4)
		parsedRows := parseValuesTuples(valuesRest, params)

		s.mu.Lock()
		tbl, exists := s.tables[tableName]
		if !exists {
			// Auto-create table if columns are specified
			var cols []string
			if colListStr != "" {
				rawCols := strings.Split(colListStr, ",")
				for _, c := range rawCols {
					cols = append(cols, sanitizeName(c))
				}
			} else if len(parsedRows) > 0 {
				for i := range parsedRows[0] {
					cols = append(cols, fmt.Sprintf("col_%d", i+1))
				}
			}
			tbl = &InMemTable{
				Name:    tableName,
				Columns: cols,
				Types:   make([]int32, len(cols)),
				Rows:    make([][]string, 0),
			}
			for i := range tbl.Types {
				tbl.Types[i] = 25
			}
			s.tables[tableName] = tbl
		}

		// Insert rows
		for _, r := range parsedRows {
			fullRow := make([]string, len(tbl.Columns))
			for i := range fullRow {
				if i < len(r) {
					fullRow[i] = r[i]
				} else {
					fullRow[i] = "NULL"
				}
			}
			tbl.Rows = append(tbl.Rows, fullRow)
		}
		s.mu.Unlock()

		rule := &Rule{
			ID:    "stateful-insert",
			Query: query,
			Tag:   fmt.Sprintf("INSERT 0 %d", len(parsedRows)),
		}

		if returningClause != "" {
			rule.Columns = tbl.Columns
			rule.Types = tbl.Types
			rule.Rows = parsedRows
		}

		return rule, true
	}

	// 5. SELECT FROM
	if matches := selectRegex.FindStringSubmatch(trimmed); len(matches) > 2 {
		colsExpr := strings.TrimSpace(matches[1])
		tableName := sanitizeName(matches[2])
		whereClause := ""
		if len(matches) > 3 {
			whereClause = strings.TrimSpace(matches[3])
		}

		s.mu.RLock()
		tbl, exists := s.tables[tableName]
		s.mu.RUnlock()

		if !exists {
			return &Rule{
				ID:    "stateful-table-missing",
				Query: query,
				Error: &MockError{
					Code:     "42P01",
					Severity: "ERROR",
					Message:  fmt.Sprintf("relation %q does not exist", tableName),
				},
			}, true
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		// Filter rows based on WHERE clause if present
		var matchedRows [][]string
		for _, row := range tbl.Rows {
			if evaluateWhere(whereClause, tbl.Columns, row, params) {
				matchedRows = append(matchedRows, row)
			}
		}

		resCols := tbl.Columns
		resTypes := tbl.Types
		resRows := matchedRows

		// Handle specific column projection
		if colsExpr != "*" && !strings.Contains(colsExpr, "count(") && !strings.Contains(colsExpr, "COUNT(") {
			colNames := strings.Split(colsExpr, ",")
			var projCols []string
			var projIndices []int
			for _, cn := range colNames {
				cleanName := sanitizeName(cn)
				projCols = append(projCols, cleanName)
				idx := findColIndex(tbl.Columns, cleanName)
				projIndices = append(projIndices, idx)
			}

			projRows := make([][]string, len(matchedRows))
			for i, r := range matchedRows {
				projRow := make([]string, len(projCols))
				for cIdx, tblIdx := range projIndices {
					if tblIdx >= 0 && tblIdx < len(r) {
						projRow[cIdx] = r[tblIdx]
					} else {
						projRow[cIdx] = "NULL"
					}
				}
				projRows[i] = projRow
			}

			resCols = projCols
			resTypes = make([]int32, len(projCols))
			for i := range resTypes {
				resTypes[i] = 25
			}
			resRows = projRows
		} else if strings.EqualFold(colsExpr, "COUNT(*)") || strings.EqualFold(colsExpr, "COUNT(1)") {
			return &Rule{
				ID:      "stateful-select-count",
				Query:   query,
				Columns: []string{"count"},
				Types:   []int32{20}, // int8
				Rows:    [][]string{{strconv.Itoa(len(matchedRows))}},
				Tag:     "SELECT 1",
			}, true
		}

		return &Rule{
			ID:      "stateful-select",
			Query:   query,
			Columns: resCols,
			Types:   resTypes,
			Rows:    resRows,
			Tag:     fmt.Sprintf("SELECT %d", len(resRows)),
		}, true
	}

	// 6. UPDATE
	if matches := updateRegex.FindStringSubmatch(trimmed); len(matches) > 2 {
		tableName := sanitizeName(matches[1])
		setExpr := strings.TrimSpace(matches[2])
		whereClause := ""
		if len(matches) > 3 {
			whereClause = strings.TrimSpace(matches[3])
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		tbl, exists := s.tables[tableName]
		if !exists {
			return &Rule{
				ID:    "stateful-table-missing",
				Query: query,
				Error: &MockError{
					Code:     "42P01",
					Severity: "ERROR",
					Message:  fmt.Sprintf("relation %q does not exist", tableName),
				},
			}, true
		}

		setAssignments := parseSetAssignments(setExpr, params)
		updatedCount := 0

		for _, row := range tbl.Rows {
			if evaluateWhere(whereClause, tbl.Columns, row, params) {
				for col, val := range setAssignments {
					idx := findColIndex(tbl.Columns, col)
					if idx >= 0 && idx < len(row) {
						row[idx] = val
					}
				}
				updatedCount++
			}
		}

		return &Rule{
			ID:    "stateful-update",
			Query: query,
			Tag:   fmt.Sprintf("UPDATE %d", updatedCount),
		}, true
	}

	// 7. DELETE
	if matches := deleteRegex.FindStringSubmatch(trimmed); len(matches) > 1 {
		tableName := sanitizeName(matches[1])
		whereClause := ""
		if len(matches) > 2 {
			whereClause = strings.TrimSpace(matches[2])
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		tbl, exists := s.tables[tableName]
		if !exists {
			return &Rule{
				ID:    "stateful-table-missing",
				Query: query,
				Error: &MockError{
					Code:     "42P01",
					Severity: "ERROR",
					Message:  fmt.Sprintf("relation %q does not exist", tableName),
				},
			}, true
		}

		newRows := make([][]string, 0)
		deletedCount := 0

		for _, row := range tbl.Rows {
			if evaluateWhere(whereClause, tbl.Columns, row, params) {
				deletedCount++
			} else {
				newRows = append(newRows, row)
			}
		}

		tbl.Rows = newRows

		return &Rule{
			ID:    "stateful-delete",
			Query: query,
			Tag:   fmt.Sprintf("DELETE %d", deletedCount),
		}, true
	}

	return nil, false
}

func sanitizeName(n string) string {
	n = strings.TrimSpace(n)
	n = strings.Trim(n, `"'` + "`")
	// If schema-qualified like "public.users", strip schema prefix
	if idx := strings.LastIndex(n, "."); idx != -1 {
		n = n[idx+1:]
	}
	return strings.ToLower(n)
}

func findColIndex(cols []string, target string) int {
	target = strings.ToLower(target)
	for i, c := range cols {
		if strings.ToLower(c) == target {
			return i
		}
	}
	return -1
}

func parseValuesTuples(valuesStr string, params [][]byte) [][]string {
	var rows [][]string
	tupleRegex := regexp.MustCompile(`\(([^)]+)\)`)
	matches := tupleRegex.FindAllStringSubmatch(valuesStr, -1)

	for _, m := range matches {
		if len(m) > 1 {
			rawVals := strings.Split(m[1], ",")
			row := make([]string, len(rawVals))
			for i, v := range rawVals {
				trimmed := strings.TrimSpace(v)
				if strings.HasPrefix(trimmed, "$") {
					paramIdx, err := strconv.Atoi(strings.TrimPrefix(trimmed, "$"))
					if err == nil && paramIdx > 0 && paramIdx <= len(params) {
						if params[paramIdx-1] == nil {
							row[i] = "NULL"
						} else {
							row[i] = string(params[paramIdx-1])
						}
						continue
					}
				}
				row[i] = strings.Trim(trimmed, `"'`)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func parseSetAssignments(setExpr string, params [][]byte) map[string]string {
	res := make(map[string]string)
	pairs := strings.Split(setExpr, ",")
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			col := sanitizeName(kv[0])
			val := strings.TrimSpace(kv[1])
			if strings.HasPrefix(val, "$") {
				paramIdx, err := strconv.Atoi(strings.TrimPrefix(val, "$"))
				if err == nil && paramIdx > 0 && paramIdx <= len(params) {
					if params[paramIdx-1] == nil {
						res[col] = "NULL"
					} else {
						res[col] = string(params[paramIdx-1])
					}
					continue
				}
			}
			res[col] = strings.Trim(val, `"'`)
		}
	}
	return res
}

func evaluateWhere(where string, cols []string, row []string, params [][]byte) bool {
	if where == "" {
		return true
	}

	// Simple equality check: col = val or col = $1
	eqParts := strings.SplitN(where, "=", 2)
	if len(eqParts) == 2 {
		targetCol := sanitizeName(eqParts[0])
		targetVal := strings.TrimSpace(eqParts[1])

		if strings.HasPrefix(targetVal, "$") {
			paramIdx, err := strconv.Atoi(strings.TrimPrefix(targetVal, "$"))
			if err == nil && paramIdx > 0 && paramIdx <= len(params) {
				if params[paramIdx-1] == nil {
					targetVal = "NULL"
				} else {
					targetVal = string(params[paramIdx-1])
				}
			}
		} else {
			targetVal = strings.Trim(targetVal, `"'`)
		}

		colIdx := findColIndex(cols, targetCol)
		if colIdx >= 0 && colIdx < len(row) {
			return strings.EqualFold(row[colIdx], targetVal)
		}
	}

	return true
}
