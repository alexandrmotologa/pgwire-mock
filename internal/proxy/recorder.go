package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
)

// TapeExchange captures a client query and its observed upstream response
type TapeExchange struct {
	Query     string     `json:"query"`
	Columns   []string   `json:"columns,omitempty"`
	Types     []int32    `json:"types,omitempty"`
	Rows      [][]string `json:"rows,omitempty"`
	Tag       string     `json:"tag,omitempty"`
	LatencyMs int        `json:"latency_ms,omitempty"`
}

// Tape is a self-contained record of captured PostgreSQL database sessions
type Tape struct {
	Version    int            `json:"version"`
	RecordedAt time.Time      `json:"recorded_at"`
	Upstream   string         `json:"upstream"`
	Exchanges  []TapeExchange `json:"exchanges"`
}

// Recorder captures live database traffic into a .pgtape file
type Recorder struct {
	upstreamAddr string
	tapePath     string
	tape         Tape
	mu           sync.Mutex
	verbose      bool
}

// NewRecorder creates a proxy recorder
func NewRecorder(upstreamAddr, tapePath string, verbose bool) *Recorder {
	return &Recorder{
		upstreamAddr: upstreamAddr,
		tapePath:     tapePath,
		tape: Tape{
			Version:    1,
			RecordedAt: time.Now().UTC(),
			Upstream:   upstreamAddr,
			Exchanges:  make([]TapeExchange, 0),
		},
		verbose: verbose,
	}
}

// ProxyConnection forwards traffic between client and upstream while logging queries
func (r *Recorder) ProxyConnection(clientConn net.Conn) {
	defer clientConn.Close()

	upstreamConn, err := net.Dial("tcp", r.upstreamAddr)
	if err != nil {
		if r.verbose {
			log.Printf("[pgwire-proxy] failed to connect to upstream %s: %v", r.upstreamAddr, err)
		}
		return
	}
	defer upstreamConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Pipe upstream -> client
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, upstreamConn)
	}()

	// Pipe client -> upstream
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstreamConn, clientConn)
	}()

	wg.Wait()
}

// Save writes the captured tape to disk
func (r *Recorder) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r.tape, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode tape: %w", err)
	}

	return os.WriteFile(r.tapePath, data, 0644)
}

// LoadTape reads a tape file from disk
func LoadTape(path string) (*Tape, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tape %s: %w", path, err)
	}

	var tape Tape
	if err := json.Unmarshal(data, &tape); err != nil {
		return nil, fmt.Errorf("failed to parse tape JSON: %w", err)
	}

	return &tape, nil
}

// ConvertTapeToRules converts a recorded tape into active mock rules
func ConvertTapeToRules(tape *Tape) []*mock.Rule {
	rules := make([]*mock.Rule, len(tape.Exchanges))
	for i, ex := range tape.Exchanges {
		rules[i] = &mock.Rule{
			ID:        fmt.Sprintf("tape-%d", i+1),
			Query:     ex.Query,
			Columns:   ex.Columns,
			Types:     ex.Types,
			Rows:      ex.Rows,
			Tag:       ex.Tag,
			LatencyMs: ex.LatencyMs,
		}
	}
	return rules
}
