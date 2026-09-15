package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
)

// Server represents a PostgreSQL Protocol Mock TCP server
type Server struct {
	addr        string
	listener    net.Listener
	engine      *mock.Engine
	bufferPool  *BufferPool
	verbose     bool
	tlsConfig   *tls.Config
	shutdown    chan struct{}
	wg          sync.WaitGroup
	activeConns int64
	queryCount  int64

	connsMu sync.Mutex
	conns   map[net.Conn]struct{}
}

// NewServer initializes a new Server
func NewServer(addr string, engine *mock.Engine, verbose bool) *Server {
	return &Server{
		addr:       addr,
		engine:     engine,
		bufferPool: NewBufferPool(),
		verbose:    verbose,
		shutdown:   make(chan struct{}),
		conns:      make(map[net.Conn]struct{}),
	}
}

// SetTLSConfig sets the TLS configuration for encrypted SSL connections
func (s *Server) SetTLSConfig(cfg *tls.Config) {
	s.tlsConfig = cfg
}

// Start binds to the configured TCP address and accepts incoming client connections
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to bind pgwire mock socket on %s: %w", s.addr, err)
	}
	s.listener = ln

	if s.verbose {
		log.Printf("[pgwire] listening for PostgreSQL clients on %s", s.listener.Addr().String())
	}

	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				if s.verbose {
					log.Printf("[pgwire] accept error: %v", err)
				}
				return
			}
		}

		s.connsMu.Lock()
		s.conns[conn] = struct{}{}
		s.connsMu.Unlock()

		s.wg.Add(1)
		go func(c net.Conn) {
			defer func() {
				s.connsMu.Lock()
				delete(s.conns, c)
				s.connsMu.Unlock()
				s.wg.Done()
			}()
			handler := NewConnHandler(c, s.engine, s.bufferPool, s.verbose, &s.activeConns, &s.queryCount, s.tlsConfig)
			handler.Handle()
		}(conn)
	}
}

// Stop gracefully terminates the listener and closes all active connections
func (s *Server) Stop() error {
	close(s.shutdown)
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}

	s.connsMu.Lock()
	for c := range s.conns {
		_ = c.Close()
	}
	s.connsMu.Unlock()

	s.wg.Wait()
	return err
}

// Addr returns the bound network address
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// ActiveConnections returns current active client connections
func (s *Server) ActiveConnections() int64 {
	return atomic.LoadInt64(&s.activeConns)
}

// TotalQueries returns the cumulative count of queries executed
func (s *Server) TotalQueries() int64 {
	return atomic.LoadInt64(&s.queryCount)
}
