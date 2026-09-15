package server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
	"github.com/alexandrmotologa/pgwire-mock/internal/protocol"
)

type portalState struct {
	stmt          *protocol.ParseMessage
	params        [][]byte
	resultFormats []int16
}

// ConnHandler processes a single client TCP connection
type ConnHandler struct {
	conn       net.Conn
	engine     *mock.Engine
	bufferPool *BufferPool
	verbose    bool
	txStatus   byte
	tlsConfig  *tls.Config

	stmts   map[string]*protocol.ParseMessage
	portals map[string]*portalState

	activeConns *int64
	queryCount  *int64
}

// NewConnHandler creates a handler for a client connection
func NewConnHandler(conn net.Conn, engine *mock.Engine, pool *BufferPool, verbose bool, activeConns *int64, queryCount *int64, tlsConfig *tls.Config) *ConnHandler {
	return &ConnHandler{
		conn:        conn,
		engine:      engine,
		bufferPool:  pool,
		verbose:     verbose,
		txStatus:    protocol.TxStatusIdle,
		tlsConfig:   tlsConfig,
		stmts:       make(map[string]*protocol.ParseMessage),
		portals:     make(map[string]*portalState),
		activeConns: activeConns,
		queryCount:  queryCount,
	}
}

// Handle executes connection lifecycle from handshake to termination
func (h *ConnHandler) Handle() {
	if h.activeConns != nil {
		atomic.AddInt64(h.activeConns, 1)
		defer atomic.AddInt64(h.activeConns, -1)
	}
	defer h.conn.Close()

	reader := bufio.NewReader(h.conn)

	// Step 1: Handshake (SSL negotiation and StartupMessage)
	newReader, err := h.handleHandshake(reader)
	if err != nil {
		if !errors.Is(err, io.EOF) && h.verbose {
			log.Printf("[pgwire] handshake error from %s: %v", h.conn.RemoteAddr(), err)
		}
		return
	}
	reader = newReader

	// Step 2: Query processing loop
	for {
		msgType, payloadLen, err := protocol.ReadMessageHeader(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if h.verbose {
				log.Printf("[pgwire] read error from %s: %v", h.conn.RemoteAddr(), err)
			}
			return
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return
		}

		shouldContinue := h.processMessage(msgType, payload)
		if !shouldContinue {
			return
		}
	}
}

func (h *ConnHandler) handleHandshake(r *bufio.Reader) (*bufio.Reader, error) {
	isSSL, startup, err := protocol.ReadStartupOrSSL(r)
	if err != nil {
		return r, err
	}

	if isSSL {
		if h.tlsConfig != nil {
			// Accept SSL: reply single byte 'S'
			if _, err := h.conn.Write([]byte{'S'}); err != nil {
				return r, err
			}
			tlsConn := tls.Server(h.conn, h.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return r, fmt.Errorf("tls handshake failed: %w", err)
			}
			h.conn = tlsConn
			r = bufio.NewReader(h.conn)

			// Read encrypted StartupMessage
			var isSSL2 bool
			isSSL2, startup, err = protocol.ReadStartupOrSSL(r)
			if err != nil {
				return r, err
			}
			if isSSL2 {
				return r, errors.New("client requested SSL twice")
			}
		} else {
			// Client requested SSL. We immediately reply 'N' so the driver falls back to plaintext.
			if err := protocol.WriteSSLRefusal(h.conn); err != nil {
				return r, err
			}
			// Read the subsequent plaintext StartupMessage
			var isSSL2 bool
			isSSL2, startup, err = protocol.ReadStartupOrSSL(r)
			if err != nil {
				return r, err
			}
			if isSSL2 {
				return r, errors.New("client requested SSL twice")
			}
		}
	}

	if h.verbose {
		log.Printf("[pgwire] client connected: user=%q db=%q addr=%s", startup.User(), startup.Database(), h.conn.RemoteAddr())
	}

	// Send AuthenticationOk
	if err := protocol.WriteAuthOK(h.conn); err != nil {
		return r, err
	}

	// Send baseline ParameterStatus messages
	if err := protocol.WriteStandardParameterStatuses(h.conn); err != nil {
		return r, err
	}

	// Send BackendKeyData (PID 1000, secret 4242)
	if err := protocol.WriteBackendKeyData(h.conn, 1000, 4242); err != nil {
		return r, err
	}

	// Send ReadyForQuery (Idle)
	return r, protocol.WriteReadyForQuery(h.conn, h.txStatus)
}

func (h *ConnHandler) processMessage(msgType byte, payload []byte) bool {
	switch msgType {
	case protocol.MsgTypeTerminate: // 'X'
		if h.verbose {
			log.Printf("[pgwire] client %s terminated session", h.conn.RemoteAddr())
		}
		return false

	case protocol.MsgTypeQuery: // 'Q'
		h.handleSimpleQuery(payload)
		return true

	case protocol.MsgTypeParse: // 'P'
		h.handleParse(payload)
		return true

	case protocol.MsgTypeBind: // 'B'
		h.handleBind(payload)
		return true

	case protocol.MsgTypeDescribe: // 'D'
		h.handleDescribe(payload)
		return true

	case protocol.MsgTypeExecute: // 'E'
		h.handleExecute(payload)
		return true

	case protocol.MsgTypeSync: // 'S'
		_ = protocol.WriteReadyForQuery(h.conn, h.txStatus)
		return true

	case protocol.MsgTypeClose: // 'C'
		h.handleClose(payload)
		return true

	case protocol.MsgTypeFlush: // 'H'
		// No-op for mock socket
		return true

	default:
		if h.verbose {
			log.Printf("[pgwire] unhandled message type '%c' (0x%x)", msgType, msgType)
		}
		_ = protocol.WriteReadyForQuery(h.conn, h.txStatus)
		return true
	}
}

func (h *ConnHandler) handleSimpleQuery(payload []byte) {
	if h.queryCount != nil {
		atomic.AddInt64(h.queryCount, 1)
	}

	qMsg, err := protocol.ReadQuery(payload)
	if err != nil {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "42601",
			Message: "syntax error in query string",
		})
		_ = protocol.WriteReadyForQuery(h.conn, h.txStatus)
		return
	}

	query := strings.TrimSpace(qMsg.Query)
	if query == "" || query == ";" {
		_ = protocol.WriteEmptyQueryResponse(h.conn)
		_ = protocol.WriteReadyForQuery(h.conn, h.txStatus)
		return
	}

	// Track transaction status
	norm := strings.ToUpper(mock.NormalizeSQL(query))
	switch norm {
	case "BEGIN", "START TRANSACTION":
		h.txStatus = protocol.TxStatusInBlock
	case "COMMIT", "ROLLBACK":
		h.txStatus = protocol.TxStatusIdle
	}

	rule := h.engine.Match(query, nil)
	h.executeRuleResponse(rule, query, nil, nil)
	_ = protocol.WriteReadyForQuery(h.conn, h.txStatus)
}

func (h *ConnHandler) handleParse(payload []byte) {
	pMsg, err := protocol.ReadParse(payload)
	if err != nil {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "42601",
			Message: err.Error(),
		})
		return
	}

	h.stmts[pMsg.StatementName] = pMsg
	_ = protocol.WriteParseComplete(h.conn)
}

func (h *ConnHandler) handleBind(payload []byte) {
	bMsg, err := protocol.ReadBind(payload)
	if err != nil {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "42P02",
			Message: err.Error(),
		})
		return
	}

	stmt, exists := h.stmts[bMsg.StatementName]
	if !exists && bMsg.StatementName != "" {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "26000",
			Message: fmt.Sprintf("prepared statement %q does not exist", bMsg.StatementName),
		})
		return
	}

	h.portals[bMsg.PortalName] = &portalState{
		stmt:          stmt,
		params:        bMsg.Parameters,
		resultFormats: bMsg.ResultFormatCodes,
	}

	_ = protocol.WriteBindComplete(h.conn)
}

func (h *ConnHandler) handleDescribe(payload []byte) {
	dMsg, err := protocol.ReadDescribe(payload)
	if err != nil {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "42601",
			Message: err.Error(),
		})
		return
	}

	var query string
	var paramOIDs []int32
	var portal *portalState

	if dMsg.TargetType == 'S' {
		stmt, exists := h.stmts[dMsg.Name]
		if !exists && dMsg.Name != "" {
			_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
				Code:    "26000",
				Message: fmt.Sprintf("prepared statement %q does not exist", dMsg.Name),
			})
			return
		}
		if stmt != nil {
			query = stmt.Query
			paramOIDs = stmt.ParamOIDs
		}
		_ = protocol.WriteParameterDescription(h.conn, paramOIDs)
	} else if dMsg.TargetType == 'P' {
		var exists bool
		portal, exists = h.portals[dMsg.Name]
		if !exists && dMsg.Name != "" {
			_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
				Code:    "34000",
				Message: fmt.Sprintf("cursor %q does not exist", dMsg.Name),
			})
			return
		}
		if portal != nil && portal.stmt != nil {
			query = portal.stmt.Query
		}
	}

	if query == "" {
		_ = protocol.WriteNoData(h.conn)
		return
	}

	rule := h.engine.Match(query, nil)
	if len(rule.Columns) == 0 {
		_ = protocol.WriteNoData(h.conn)
		return
	}

	fields := make([]protocol.FieldDescription, len(rule.Columns))
	for i, col := range rule.Columns {
		oid := int32(25) // default text
		if i < len(rule.Types) && rule.Types[i] != 0 {
			oid = rule.Types[i]
		}
		fmtCode := protocol.FormatText
		if portal != nil {
			if len(portal.resultFormats) == 1 && portal.resultFormats[0] == protocol.FormatBinary {
				fmtCode = protocol.FormatBinary
			} else if i < len(portal.resultFormats) && portal.resultFormats[i] == protocol.FormatBinary {
				fmtCode = protocol.FormatBinary
			}
		}
		fields[i] = protocol.FieldDescription{
			Name:         col,
			TableOID:     0,
			ColumnAttr:   int16(i + 1),
			DataTypeOID:  oid,
			DataTypeSize: protocol.DefaultTypeSize(oid),
			TypeModifier: -1,
			FormatCode:   fmtCode,
		}
	}

	_ = protocol.WriteRowDescription(h.conn, fields)
}

func (h *ConnHandler) handleExecute(payload []byte) {
	if h.queryCount != nil {
		atomic.AddInt64(h.queryCount, 1)
	}

	eMsg, err := protocol.ReadExecute(payload)
	if err != nil {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "42601",
			Message: err.Error(),
		})
		return
	}

	portal, exists := h.portals[eMsg.PortalName]
	if !exists && eMsg.PortalName != "" {
		_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
			Code:    "34000",
			Message: fmt.Sprintf("cursor %q does not exist", eMsg.PortalName),
		})
		return
	}

	query := ""
	var params [][]byte
	var resultFormats []int16
	if portal != nil {
		if portal.stmt != nil {
			query = portal.stmt.Query
		}
		params = portal.params
		resultFormats = portal.resultFormats
	}

	rule := h.engine.Match(query, params)
	h.executeRuleResponse(rule, query, params, resultFormats)
}

func (h *ConnHandler) handleClose(payload []byte) {
	cMsg, err := protocol.ReadClose(payload)
	if err != nil {
		_ = protocol.WriteCloseComplete(h.conn)
		return
	}

	if cMsg.TargetType == 'S' {
		delete(h.stmts, cMsg.Name)
	} else if cMsg.TargetType == 'P' {
		delete(h.portals, cMsg.Name)
	}
	_ = protocol.WriteCloseComplete(h.conn)
}

func (h *ConnHandler) executeRuleResponse(rule *mock.Rule, query string, params [][]byte, resultFormats []int16) {
	// Fault injection: drop connection
	if rule.DropConnection {
		if h.verbose {
			log.Printf("[pgwire-chaos] dropping connection for query: %s", query)
		}
		_ = h.conn.Close()
		return
	}

	// Fault injection: artificial latency & jitter
	totalLatency := rule.LatencyMs
	if rule.JitterMs > 0 {
		totalLatency += rand.Intn(rule.JitterMs)
	}
	if totalLatency > 0 {
		time.Sleep(time.Duration(totalLatency) * time.Millisecond)
	}

	// Fault injection: error condition (respecting fail_after_n counter)
	if rule.Error != nil {
		shouldFail := true
		if rule.FailAfterN > 0 && rule.Hits() <= int64(rule.FailAfterN) {
			shouldFail = false
		}
		if shouldFail {
			_ = protocol.WriteErrorResponse(h.conn, protocol.ErrorResponse{
				Severity: rule.Error.Severity,
				Code:     rule.Error.Code,
				Message:  rule.Error.Message,
				Detail:   rule.Error.Detail,
				Hint:     rule.Error.Hint,
			})
			return
		}
	}

	// Success response: RowDescription if columns exist
	if len(rule.Columns) > 0 {
		fields := make([]protocol.FieldDescription, len(rule.Columns))
		for i, col := range rule.Columns {
			oid := int32(25) // default text
			if i < len(rule.Types) && rule.Types[i] != 0 {
				oid = rule.Types[i]
			}
			fmtCode := protocol.FormatText
			if len(resultFormats) == 1 && resultFormats[0] == protocol.FormatBinary {
				fmtCode = protocol.FormatBinary
			} else if i < len(resultFormats) && resultFormats[i] == protocol.FormatBinary {
				fmtCode = protocol.FormatBinary
			}
			fields[i] = protocol.FieldDescription{
				Name:         col,
				TableOID:     0,
				ColumnAttr:   int16(i + 1),
				DataTypeOID:  oid,
				DataTypeSize: protocol.DefaultTypeSize(oid),
				TypeModifier: -1,
				FormatCode:   fmtCode,
			}
		}
		_ = protocol.WriteRowDescription(h.conn, fields)

		// Emit DataRows
		for _, row := range rule.Rows {
			rowBytes := make([][]byte, len(rule.Columns))
			for colIdx := range rule.Columns {
				if colIdx < len(row) {
					val := row[colIdx]
					if val == "NULL" {
						rowBytes[colIdx] = nil
					} else {
						// Dynamic template substitution
						rendered := mock.RenderTemplate(val, params)

						// Determine format code (binary vs text)
						isBinary := false
						if len(resultFormats) == 1 && resultFormats[0] == protocol.FormatBinary {
							isBinary = true
						} else if colIdx < len(resultFormats) && resultFormats[colIdx] == protocol.FormatBinary {
							isBinary = true
						}

						if isBinary {
							rowBytes[colIdx] = protocol.EncodeBinaryValue(fields[colIdx].DataTypeOID, rendered)
						} else {
							rowBytes[colIdx] = []byte(rendered)
						}
					}
				} else {
					rowBytes[colIdx] = nil
				}
			}
			_ = protocol.WriteDataRow(h.conn, rowBytes)
		}
	}

	tag := rule.Tag
	if tag == "" {
		if len(rule.Columns) > 0 {
			tag = fmt.Sprintf("SELECT %d", len(rule.Rows))
		} else {
			tag = "OK"
		}
	}
	_ = protocol.WriteCommandComplete(h.conn, tag)

	if h.verbose {
		log.Printf("[pgwire] matched rule %q -> tag %q (rows=%d)", rule.ID, tag, len(rule.Rows))
	}
}
