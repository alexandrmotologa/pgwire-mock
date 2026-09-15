package mock

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// QueryLog records an executed query for verification and assertions
type QueryLog struct {
	Timestamp     time.Time `json:"timestamp"`
	Query         string    `json:"query"`
	Params        []string  `json:"params,omitempty"`
	MatchedRuleID string    `json:"matched_rule_id,omitempty"`
}

// Engine manages mock rules and matches incoming queries
type Engine struct {
	mu      sync.RWMutex
	rules   []*Rule
	matcher *Matcher

	logMu sync.RWMutex
	logs  []QueryLog
}

// NewEngine creates a new mock Engine
func NewEngine(initialRules []*Rule) *Engine {
	return &Engine{
		rules:   initialRules,
		matcher: NewMatcher(),
		logs:    make([]QueryLog, 0),
	}
}

// AddRule appends a rule to the active registry
func (e *Engine) AddRule(r *Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, r)
}

// PrependRule adds a high-priority rule to the beginning
func (e *Engine) PrependRule(r *Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append([]*Rule{r}, e.rules...)
}

// DeleteRule removes a rule by ID
func (e *Engine) DeleteRule(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == id {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return true
		}
	}
	return false
}

// ClearRules resets all custom rules
func (e *Engine) ClearRules() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = make([]*Rule, 0)
}

// GetRules returns a copy of registered rules
func (e *Engine) GetRules() []*Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	copied := make([]*Rule, len(e.rules))
	copy(copied, e.rules)
	return copied
}

// LogQuery records an executed query
func (e *Engine) LogQuery(query string, params [][]byte, matchedID string) {
	paramStrings := make([]string, len(params))
	for i, p := range params {
		if p == nil {
			paramStrings[i] = "NULL"
		} else {
			paramStrings[i] = string(p)
		}
	}

	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.logs = append(e.logs, QueryLog{
		Timestamp:     time.Now().UTC(),
		Query:         query,
		Params:        paramStrings,
		MatchedRuleID: matchedID,
	})
}

// GetLogs returns all recorded queries
func (e *Engine) GetLogs() []QueryLog {
	e.logMu.RLock()
	defer e.logMu.RUnlock()
	copied := make([]QueryLog, len(e.logs))
	copy(copied, e.logs)
	return copied
}

// ResetLogs clears query history
func (e *Engine) ResetLogs() {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	e.logs = make([]QueryLog, 0)
}

// Match searches for a matching rule or generates a standard default fallback
func (e *Engine) Match(query string, params [][]byte) *Rule {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, r := range rules {
		if e.matcher.Matches(r, query, params) {
			r.IncrementHits()
			e.LogQuery(query, params, r.ID)
			return r
		}
	}

	// Default system fallback handler
	fallback := e.matchDefaultSystemQuery(query)
	if fallback != nil {
		e.LogQuery(query, params, "builtin-system")
		return fallback
	}

	// Final generic fallback for unmatched queries
	e.LogQuery(query, params, "default-fallback")
	return &Rule{
		ID:      "default-fallback",
		Query:   query,
		Columns: []string{},
		Rows:    [][]string{},
		Tag:     detectCommandTag(query),
	}
}

func (e *Engine) matchDefaultSystemQuery(query string) *Rule {
	norm := strings.ToUpper(NormalizeSQL(query))

	switch {
	case norm == "SELECT 1" || norm == "SELECT 1 AS ONE":
		return &Rule{
			ID:      "builtin-select-1",
			Query:   query,
			Columns: []string{"?column?"},
			Types:   []int32{23}, // int4
			Rows:    [][]string{{"1"}},
			Tag:     "SELECT 1",
		}
	case strings.HasPrefix(norm, "SET "):
		return &Rule{
			ID:      "builtin-set",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "SET",
		}
	case strings.HasPrefix(norm, "SHOW "):
		paramName := strings.TrimPrefix(norm, "SHOW ")
		val := "on"
		if strings.EqualFold(paramName, "SERVER_VERSION") {
			val = "16.2 (PGWire-Mock)"
		} else if strings.EqualFold(paramName, "CLIENT_ENCODING") {
			val = "UTF8"
		}
		return &Rule{
			ID:      "builtin-show",
			Query:   query,
			Columns: []string{strings.ToLower(paramName)},
			Types:   []int32{25},
			Rows:    [][]string{{val}},
			Tag:     "SHOW",
		}
	case norm == "BEGIN" || norm == "START TRANSACTION":
		return &Rule{
			ID:      "builtin-begin",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "BEGIN",
		}
	case norm == "COMMIT":
		return &Rule{
			ID:      "builtin-commit",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "COMMIT",
		}
	case norm == "ROLLBACK":
		return &Rule{
			ID:      "builtin-rollback",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "ROLLBACK",
		}
	case norm == "DISCARD ALL":
		return &Rule{
			ID:      "builtin-discard",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "DISCARD ALL",
		}
	case strings.HasPrefix(norm, "UNLISTEN "):
		return &Rule{
			ID:      "builtin-unlisten",
			Query:   query,
			Columns: []string{},
			Rows:    [][]string{},
			Tag:     "UNLISTEN",
		}
	}

	return nil
}

func detectCommandTag(query string) string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	parts := strings.Fields(upper)
	if len(parts) == 0 {
		return "OK"
	}
	switch parts[0] {
	case "SELECT":
		return "SELECT 0"
	case "INSERT":
		return "INSERT 0 1"
	case "UPDATE":
		return "UPDATE 0"
	case "DELETE":
		return "DELETE 0"
	case "CREATE":
		if len(parts) > 1 {
			return fmt.Sprintf("CREATE %s", parts[1])
		}
		return "CREATE"
	case "DROP":
		if len(parts) > 1 {
			return fmt.Sprintf("DROP %s", parts[1])
		}
		return "DROP"
	case "ALTER":
		return "ALTER"
	default:
		return parts[0]
	}
}
