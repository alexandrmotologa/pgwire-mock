package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
	"github.com/alexandrmotologa/pgwire-mock/internal/server"
)

// Server provides an embedded HTTP REST and Assertion API for test runners
type Server struct {
	addr       string
	engine     *mock.Engine
	pgServer   *server.Server
	httpServer *http.Server
	listener   net.Listener
	verbose    bool
	mu         sync.Mutex
}

// NewServer creates a new Admin API server
func NewServer(addr string, engine *mock.Engine, pgServer *server.Server, verbose bool) *Server {
	return &Server{
		addr:     addr,
		engine:   engine,
		pgServer: pgServer,
		verbose:  verbose,
	}
}

// Start runs the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/rules", s.handleGetRules)
	mux.HandleFunc("POST /api/rules", s.handleAddRule)
	mux.HandleFunc("DELETE /api/rules/", s.handleDeleteRule)
	mux.HandleFunc("GET /api/queries", s.handleGetQueries)
	mux.HandleFunc("POST /api/assert", s.handleAssert)
	mux.HandleFunc("POST /api/reset", s.handleReset)

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to bind admin API on %s: %w", s.addr, err)
	}
	s.listener = ln
	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	if s.verbose {
		log.Printf("[pgwire-admin] listening on http://%s", ln.Addr().String())
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			if s.verbose {
				log.Printf("[pgwire-admin] error: %v", err)
			}
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Addr returns the bound network address
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "1.0.0",
		"service": "pgwire-mock",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var activeConns, totalQueries int64
	if s.pgServer != nil {
		activeConns = s.pgServer.ActiveConnections()
		totalQueries = s.pgServer.TotalQueries()
	}
	rulesCount := len(s.engine.GetRules())

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP pgwire_active_connections Current active PostgreSQL client connections\n")
	fmt.Fprintf(w, "# TYPE pgwire_active_connections gauge\n")
	fmt.Fprintf(w, "pgwire_active_connections %d\n", activeConns)

	fmt.Fprintf(w, "# HELP pgwire_total_queries Cumulative total queries processed\n")
	fmt.Fprintf(w, "# TYPE pgwire_total_queries counter\n")
	fmt.Fprintf(w, "pgwire_total_queries %d\n", totalQueries)

	fmt.Fprintf(w, "# HELP pgwire_mock_rules_total Active mock rules registered\n")
	fmt.Fprintf(w, "# TYPE pgwire_mock_rules_total gauge\n")
	fmt.Fprintf(w, "pgwire_mock_rules_total %d\n", rulesCount)
}

func (s *Server) handleGetRules(w http.ResponseWriter, r *http.Request) {
	rules := s.engine.GetRules()
	respondJSON(w, http.StatusOK, map[string]any{
		"rules": rules,
		"count": len(rules),
	})
}

func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var rule mock.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON payload: %v", err), http.StatusBadRequest)
		return
	}

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-dyn-%d", time.Now().UnixNano())
	}
	if rule.Tag == "" {
		if len(rule.Columns) > 0 {
			rule.Tag = fmt.Sprintf("SELECT %d", len(rule.Rows))
		} else {
			rule.Tag = "OK"
		}
	}
	if len(rule.Types) < len(rule.Columns) {
		newTypes := make([]int32, len(rule.Columns))
		for i := range rule.Columns {
			if i < len(rule.Types) && rule.Types[i] != 0 {
				newTypes[i] = rule.Types[i]
			} else {
				newTypes[i] = 25 // text
			}
		}
		rule.Types = newTypes
	}

	s.engine.PrependRule(&rule)
	respondJSON(w, http.StatusCreated, rule)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/rules/")
	if id == "" {
		http.Error(w, "missing rule ID in path", http.StatusBadRequest)
		return
	}

	deleted := s.engine.DeleteRule(id)
	if !deleted {
		http.Error(w, fmt.Sprintf("rule %q not found", id), http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) handleGetQueries(w http.ResponseWriter, r *http.Request) {
	logs := s.engine.GetLogs()
	respondJSON(w, http.StatusOK, map[string]any{
		"queries": logs,
		"count":   len(logs),
	})
}

// AssertRequest specifies criteria for automated test assertions
type AssertRequest struct {
	Query       string   `json:"query"`                  // Query or substring to assert
	Count       *int     `json:"count,omitempty"`        // Expected exact count of executions
	MinCount    *int     `json:"min_count,omitempty"`    // Minimum executions required
	ExactParams []string `json:"exact_params,omitempty"` // Exact parameters for at least one call
}

func (s *Server) handleAssert(w http.ResponseWriter, r *http.Request) {
	var req AssertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid assert payload: %v", err), http.StatusBadRequest)
		return
	}

	normExpected := mock.NormalizeSQL(req.Query)
	logs := s.engine.GetLogs()

	matchingCalls := 0
	foundExactParams := false

	for _, l := range logs {
		normActual := mock.NormalizeSQL(l.Query)
		matches := false
		if req.Query == "" || strings.EqualFold(normExpected, normActual) || strings.Contains(strings.ToUpper(normActual), strings.ToUpper(normExpected)) {
			matches = true
		}

		if matches {
			matchingCalls++
			if len(req.ExactParams) > 0 {
				if len(req.ExactParams) == len(l.Params) {
					paramsMatch := true
					for i, p := range req.ExactParams {
						if p != l.Params[i] {
							paramsMatch = false
							break
						}
					}
					if paramsMatch {
						foundExactParams = true
					}
				}
			}
		}
	}

	if req.Count != nil && matchingCalls != *req.Count {
		respondJSON(w, http.StatusOK, map[string]any{
			"passed":       false,
			"actual_count": matchingCalls,
			"error":        fmt.Sprintf("assertion failed: expected %d calls for %q, but got %d", *req.Count, req.Query, matchingCalls),
		})
		return
	}

	if req.MinCount != nil && matchingCalls < *req.MinCount {
		respondJSON(w, http.StatusOK, map[string]any{
			"passed":       false,
			"actual_count": matchingCalls,
			"error":        fmt.Sprintf("assertion failed: expected at least %d calls for %q, but got %d", *req.MinCount, req.Query, matchingCalls),
		})
		return
	}

	if len(req.ExactParams) > 0 && !foundExactParams {
		respondJSON(w, http.StatusOK, map[string]any{
			"passed":       false,
			"actual_count": matchingCalls,
			"error":        fmt.Sprintf("assertion failed: parameters %v were not matched in any execution of %q", req.ExactParams, req.Query),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"passed":       true,
		"actual_count": matchingCalls,
	})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.engine.ResetLogs()
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "cleared",
	})
}

func respondJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
