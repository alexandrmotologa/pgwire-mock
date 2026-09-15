package mock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// MockError defines an error to return when a rule matches
type MockError struct {
	Code     string `json:"code" yaml:"code"`         // SQLSTATE code, e.g. "42P01", "40001"
	Message  string `json:"message" yaml:"message"`   // Error description
	Severity string `json:"severity" yaml:"severity"` // "ERROR", "FATAL", etc.
	Detail   string `json:"detail" yaml:"detail"`
	Hint     string `json:"hint" yaml:"hint"`
}

// Rule defines a mock match condition and its associated response
type Rule struct {
	ID             string     `json:"id,omitempty" yaml:"id,omitempty"`
	Query          string     `json:"query" yaml:"query"`                   // Raw or normalized SQL query
	Pattern        string     `json:"pattern,omitempty" yaml:"pattern,omitempty"` // Regex pattern
	Params         []string   `json:"params,omitempty" yaml:"params,omitempty"`   // Expected parameter bindings ($1, $2)
	Columns        []string   `json:"columns,omitempty" yaml:"columns,omitempty"` // Column names
	Types          []int32    `json:"types,omitempty" yaml:"types,omitempty"`     // Data type OIDs
	Rows           [][]string `json:"rows,omitempty" yaml:"rows,omitempty"`       // Row string values
	Tag            string     `json:"tag,omitempty" yaml:"tag,omitempty"`         // Command tag, e.g. "SELECT 1"
	Error          *MockError `json:"error,omitempty" yaml:"error,omitempty"`     // Injected error
	LatencyMs      int        `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	JitterMs       int        `json:"jitter_ms,omitempty" yaml:"jitter_ms,omitempty"`
	DropConnection bool       `json:"drop_connection,omitempty" yaml:"drop_connection,omitempty"`
	FailAfterN     int        `json:"fail_after_n,omitempty" yaml:"fail_after_n,omitempty"`

	// Runtime state
	hits int64
}

// IncrementHits atomically records a match
func (r *Rule) IncrementHits() int64 {
	return atomic.AddInt64(&r.hits, 1)
}

// Hits returns current execution count
func (r *Rule) Hits() int64 {
	return atomic.LoadInt64(&r.hits)
}

// RulesFile wraps a collection of rules in YAML/JSON
type RulesFile struct {
	Rules []*Rule `json:"rules" yaml:"rules"`
}

// LoadRulesFile loads mock rules from a YAML or JSON file
func LoadRulesFile(path string) ([]*Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules file %s: %w", path, err)
	}

	var rf RulesFile
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		if err := json.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("failed to parse JSON rules: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("failed to parse YAML rules: %w", err)
		}
	}

	for i, r := range rf.Rules {
		if r.ID == "" {
			r.ID = fmt.Sprintf("rule-%d", i+1)
		}
		// Populate default command tag if missing
		if r.Tag == "" {
			if len(r.Columns) > 0 {
				r.Tag = fmt.Sprintf("SELECT %d", len(r.Rows))
			} else {
				r.Tag = "OK"
			}
		}
		// Default types to text (25) if not explicitly set
		if len(r.Types) < len(r.Columns) {
			newTypes := make([]int32, len(r.Columns))
			for colIdx := range r.Columns {
				if colIdx < len(r.Types) && r.Types[colIdx] != 0 {
					newTypes[colIdx] = r.Types[colIdx]
				} else {
					newTypes[colIdx] = 25 // OIDText
				}
			}
			r.Types = newTypes
		}
	}

	return rf.Rules, nil
}
