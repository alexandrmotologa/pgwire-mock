package mock

import (
	"regexp"
	"strings"
	"sync"
)

var (
	spaceRegex = regexp.MustCompile(`\s+`)
	regexCache = sync.Map{}
)

// NormalizeSQL standardizes SQL queries for robust comparison.
// It trims whitespace, collapses internal whitespace, and removes trailing semicolons.
func NormalizeSQL(sql string) string {
	s := strings.TrimSpace(sql)
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimSpace(s)
	return spaceRegex.ReplaceAllString(s, " ")
}

// Matcher evaluates whether an incoming query and its parameters match a Rule
type Matcher struct{}

// NewMatcher creates a new Matcher instance
func NewMatcher() *Matcher {
	return &Matcher{}
}

// Matches checks if the rule matches the provided query and parameter values
func (m *Matcher) Matches(rule *Rule, incomingQuery string, incomingParams [][]byte) bool {
	// 1. Parameter count and value check (if rule specifies params)
	if len(rule.Params) > 0 {
		if len(rule.Params) != len(incomingParams) {
			return false
		}
		for i, expected := range rule.Params {
			if expected == "*" || expected == "ANY" {
				continue
			}
			actual := incomingParams[i]
			if expected == "NULL" {
				if actual != nil {
					return false
				}
				continue
			}
			if actual == nil {
				return false
			}
			if expected != string(actual) {
				return false
			}
		}
	}

	// 2. Regex pattern match
	if rule.Pattern != "" {
		re, err := getOrCompileRegex(rule.Pattern)
		if err == nil {
			if re.MatchString(incomingQuery) {
				return true
			}
		}
	}

	// 3. Normalized SQL match
	normalizedRule := NormalizeSQL(rule.Query)
	normalizedIncoming := NormalizeSQL(incomingQuery)

	if strings.EqualFold(normalizedRule, normalizedIncoming) {
		return true
	}

	// 4. Wildcard prefix match if rule ends with "..." or "%"
	if strings.HasSuffix(normalizedRule, "...") {
		prefix := strings.TrimSuffix(normalizedRule, "...")
		if strings.HasPrefix(strings.ToUpper(normalizedIncoming), strings.ToUpper(prefix)) {
			return true
		}
	}
	if strings.HasSuffix(normalizedRule, "%") {
		prefix := strings.TrimSuffix(normalizedRule, "%")
		if strings.HasPrefix(strings.ToUpper(normalizedIncoming), strings.ToUpper(prefix)) {
			return true
		}
	}

	return false
}

func getOrCompileRegex(pattern string) (*regexp.Regexp, error) {
	if val, ok := regexCache.Load(pattern); ok {
		return val.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}
