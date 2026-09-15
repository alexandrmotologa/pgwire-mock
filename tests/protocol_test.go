package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
	"github.com/alexandrmotologa/pgwire-mock/internal/server"
)

func startTestServer(t *testing.T, rules []*mock.Rule) (*server.Server, string) {
	eng := mock.NewEngine(rules)
	srv := server.NewServer("127.0.0.1:0", eng, false)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	return srv, srv.Addr()
}

func connectAndHandshake(t *testing.T, addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}

	// 1. Send SSLRequest
	var sslReq [8]byte
	binary.BigEndian.PutUint32(sslReq[0:4], 8)
	binary.BigEndian.PutUint32(sslReq[4:8], 80877103)
	if _, err := conn.Write(sslReq[:]); err != nil {
		t.Fatalf("failed to send SSLRequest: %v", err)
	}

	var sslReply [1]byte
	if _, err := io.ReadFull(conn, sslReply[:]); err != nil {
		t.Fatalf("failed to read SSL response: %v", err)
	}
	if sslReply[0] != 'N' {
		t.Fatalf("expected 'N' SSL refusal, got %c", sslReply[0])
	}

	// 2. Send StartupMessage
	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, uint32(196608))
	body.WriteString("user\x00postgres\x00database\x00testdb\x00\x00")

	var packet bytes.Buffer
	_ = binary.Write(&packet, binary.BigEndian, uint32(4+body.Len()))
	packet.Write(body.Bytes())

	if _, err := conn.Write(packet.Bytes()); err != nil {
		t.Fatalf("failed to send StartupMessage: %v", err)
	}

	// 3. Read until ReadyForQuery ('Z')
	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(conn, typeBuf[:]); err != nil {
			t.Fatalf("handshake error: %v", err)
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			t.Fatalf("handshake length error: %v", err)
		}
		pLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		payload := make([]byte, pLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			t.Fatalf("handshake payload error: %v", err)
		}

		if typeBuf[0] == 'Z' {
			break
		}
	}

	return conn
}

func TestEndToEndSimpleQuery(t *testing.T) {
	rules := []*mock.Rule{
		{
			ID:      "users-list",
			Query:   "SELECT id, name FROM users",
			Columns: []string{"id", "name"},
			Types:   []int32{23, 25},
			Rows: [][]string{
				{"1", "Alice"},
				{"2", "Bob"},
			},
			Tag: "SELECT 2",
		},
	}

	srv, addr := startTestServer(t, rules)
	defer srv.Stop()

	conn := connectAndHandshake(t, addr)
	defer conn.Close()

	// Send Simple Query
	sql := "SELECT id, name FROM users"
	var qBuf bytes.Buffer
	qBuf.WriteByte('Q')
	_ = binary.Write(&qBuf, binary.BigEndian, uint32(4+len(sql)+1))
	qBuf.WriteString(sql)
	qBuf.WriteByte(0)

	if _, err := conn.Write(qBuf.Bytes()); err != nil {
		t.Fatalf("failed to send query: %v", err)
	}

	gotRowDescription := false
	dataRowsCount := 0
	gotCommandComplete := false

	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(conn, typeBuf[:]); err != nil {
			t.Fatalf("failed to read response: %v", err)
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			t.Fatalf("failed to read response length: %v", err)
		}
		pLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		payload := make([]byte, pLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			t.Fatalf("failed to read response payload: %v", err)
		}

		switch typeBuf[0] {
		case 'T':
			gotRowDescription = true
		case 'D':
			dataRowsCount++
		case 'C':
			gotCommandComplete = true
			tag := string(payload[:len(payload)-1])
			if tag != "SELECT 2" {
				t.Errorf("expected tag 'SELECT 2', got %q", tag)
			}
		case 'Z':
			goto done
		}
	}

done:
	if !gotRowDescription {
		t.Errorf("missing RowDescription in query response")
	}
	if dataRowsCount != 2 {
		t.Errorf("expected 2 DataRows, got %d", dataRowsCount)
	}
	if !gotCommandComplete {
		t.Errorf("missing CommandComplete")
	}
}

func TestEndToEndExtendedQuery(t *testing.T) {
	rules := []*mock.Rule{
		{
			ID:      "user-by-id",
			Query:   "SELECT name FROM users WHERE id = $1",
			Params:  []string{"42"},
			Columns: []string{"name"},
			Types:   []int32{25},
			Rows:    [][]string{{"DeepThought"}},
			Tag:     "SELECT 1",
		},
	}

	srv, addr := startTestServer(t, rules)
	defer srv.Stop()

	conn := connectAndHandshake(t, addr)
	defer conn.Close()

	// 1. Parse ('P')
	stmtName := "user_stmt"
	query := "SELECT name FROM users WHERE id = $1"
	var pPayload bytes.Buffer
	pPayload.WriteString(stmtName + "\x00")
	pPayload.WriteString(query + "\x00")
	_ = binary.Write(&pPayload, binary.BigEndian, int16(0)) // 0 param types

	var pPacket bytes.Buffer
	pPacket.WriteByte('P')
	_ = binary.Write(&pPacket, binary.BigEndian, uint32(4+pPayload.Len()))
	pPacket.Write(pPayload.Bytes())
	if _, err := conn.Write(pPacket.Bytes()); err != nil {
		t.Fatalf("failed to write Parse: %v", err)
	}

	// Read ParseComplete ('1')
	var tBuf [1]byte
	var lBuf [4]byte
	_, _ = io.ReadFull(conn, tBuf[:])
	_, _ = io.ReadFull(conn, lBuf[:])
	if tBuf[0] != '1' {
		t.Fatalf("expected ParseComplete '1', got %c", tBuf[0])
	}

	// 2. Bind ('B') with param "42"
	portalName := "user_portal"
	var bPayload bytes.Buffer
	bPayload.WriteString(portalName + "\x00")
	bPayload.WriteString(stmtName + "\x00")
	_ = binary.Write(&bPayload, binary.BigEndian, int16(0)) // 0 format codes
	_ = binary.Write(&bPayload, binary.BigEndian, int16(1)) // 1 param
	paramVal := []byte("42")
	_ = binary.Write(&bPayload, binary.BigEndian, int32(len(paramVal)))
	bPayload.Write(paramVal)
	_ = binary.Write(&bPayload, binary.BigEndian, int16(0)) // 0 result format codes

	var bPacket bytes.Buffer
	bPacket.WriteByte('B')
	_ = binary.Write(&bPacket, binary.BigEndian, uint32(4+bPayload.Len()))
	bPacket.Write(bPayload.Bytes())
	if _, err := conn.Write(bPacket.Bytes()); err != nil {
		t.Fatalf("failed to write Bind: %v", err)
	}

	// Read BindComplete ('2')
	_, _ = io.ReadFull(conn, tBuf[:])
	_, _ = io.ReadFull(conn, lBuf[:])
	if tBuf[0] != '2' {
		t.Fatalf("expected BindComplete '2', got %c", tBuf[0])
	}

	// 3. Execute ('E') and Sync ('S')
	var ePayload bytes.Buffer
	ePayload.WriteString(portalName + "\x00")
	_ = binary.Write(&ePayload, binary.BigEndian, int32(0)) // max rows 0

	var ePacket bytes.Buffer
	ePacket.WriteByte('E')
	_ = binary.Write(&ePacket, binary.BigEndian, uint32(4+ePayload.Len()))
	ePacket.Write(ePayload.Bytes())

	// Sync packet
	ePacket.Write([]byte{'S', 0, 0, 0, 4})

	if _, err := conn.Write(ePacket.Bytes()); err != nil {
		t.Fatalf("failed to write Execute+Sync: %v", err)
	}

	// Read DataRow, CommandComplete, ReadyForQuery
	gotDataRow := false
	for {
		_, err := io.ReadFull(conn, tBuf[:])
		if err != nil {
			t.Fatalf("error reading execution response: %v", err)
		}
		_, err = io.ReadFull(conn, lBuf[:])
		if err != nil {
			t.Fatalf("error reading execution length: %v", err)
		}
		pLen := binary.BigEndian.Uint32(lBuf[:]) - 4
		p := make([]byte, pLen)
		_, _ = io.ReadFull(conn, p)

		if tBuf[0] == 'T' {
			// RowDescription
		} else if tBuf[0] == 'D' {
			gotDataRow = true
		} else if tBuf[0] == 'C' {
			// CommandComplete
		} else if tBuf[0] == 'Z' {
			break
		}
	}

	if !gotDataRow {
		t.Errorf("expected DataRow for parameterized query execution")
	}
}

func TestFaultInjectionDropConnection(t *testing.T) {
	rules := []*mock.Rule{
		{
			ID:             "drop-test",
			Query:          "SELECT drop_me()",
			DropConnection: true,
		},
	}

	srv, addr := startTestServer(t, rules)
	defer srv.Stop()

	conn := connectAndHandshake(t, addr)
	defer conn.Close()

	sql := "SELECT drop_me()"
	var qBuf bytes.Buffer
	qBuf.WriteByte('Q')
	_ = binary.Write(&qBuf, binary.BigEndian, uint32(4+len(sql)+1))
	qBuf.WriteString(sql)
	qBuf.WriteByte(0)

	_, _ = conn.Write(qBuf.Bytes())

	// Next read should return EOF because server closed connection
	var b [1]byte
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err := conn.Read(b[:])
	if err == nil {
		t.Errorf("expected connection drop, but read succeeded")
	}
}

func TestEndToEndTLSConnection(t *testing.T) {
	tlsCfg, err := server.LoadOrGenerateTLSConfig("", "")
	if err != nil {
		t.Fatalf("failed to generate TLS config: %v", err)
	}

	eng := mock.NewEngine(nil)
	srv := server.NewServer("127.0.0.1:0", eng, false)
	srv.SetTLSConfig(tlsCfg)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start TLS server: %v", err)
	}
	defer srv.Stop()

	rawConn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("failed to connect to TLS server: %v", err)
	}
	defer rawConn.Close()

	// 1. Send SSLRequest
	var sslReq [8]byte
	binary.BigEndian.PutUint32(sslReq[0:4], 8)
	binary.BigEndian.PutUint32(sslReq[4:8], 80877103)
	if _, err := rawConn.Write(sslReq[:]); err != nil {
		t.Fatalf("failed to send SSLRequest: %v", err)
	}

	var sslReply [1]byte
	if _, err := io.ReadFull(rawConn, sslReply[:]); err != nil {
		t.Fatalf("failed to read SSL response: %v", err)
	}
	if sslReply[0] != 'S' {
		t.Fatalf("expected 'S' SSL acceptance, got %c", sslReply[0])
	}

	// 2. Wrap connection with client TLS
	tlsConn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("client TLS handshake failed: %v", err)
	}

	// 3. Send StartupMessage over TLS
	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, uint32(196608))
	body.WriteString("user\x00postgres\x00database\x00testdb\x00\x00")

	var packet bytes.Buffer
	_ = binary.Write(&packet, binary.BigEndian, uint32(4+body.Len()))
	packet.Write(body.Bytes())

	if _, err := tlsConn.Write(packet.Bytes()); err != nil {
		t.Fatalf("failed to send StartupMessage over TLS: %v", err)
	}

	// Read until ReadyForQuery ('Z')
	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(tlsConn, typeBuf[:]); err != nil {
			t.Fatalf("TLS handshake error: %v", err)
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(tlsConn, lenBuf[:]); err != nil {
			t.Fatalf("TLS handshake length error: %v", err)
		}
		pLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		payload := make([]byte, pLen)
		if _, err := io.ReadFull(tlsConn, payload); err != nil {
			t.Fatalf("TLS handshake payload error: %v", err)
		}
		if typeBuf[0] == 'Z' {
			break
		}
	}

	// 4. Query over TLS
	sql := "SELECT 1"
	var qBuf bytes.Buffer
	qBuf.WriteByte('Q')
	_ = binary.Write(&qBuf, binary.BigEndian, uint32(4+len(sql)+1))
	qBuf.WriteString(sql)
	qBuf.WriteByte(0)
	_, _ = tlsConn.Write(qBuf.Bytes())

	gotComplete := false
	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(tlsConn, typeBuf[:]); err != nil {
			break
		}
		var lenBuf [4]byte
		_, _ = io.ReadFull(tlsConn, lenBuf[:])
		pLen := binary.BigEndian.Uint32(lenBuf[:]) - 4
		p := make([]byte, pLen)
		_, _ = io.ReadFull(tlsConn, p)

		if typeBuf[0] == 'C' {
			gotComplete = true
		} else if typeBuf[0] == 'Z' {
			break
		}
	}

	if !gotComplete {
		t.Errorf("expected CommandComplete over TLS connection")
	}
}

func TestEndToEndDynamicTemplating(t *testing.T) {
	rules := []*mock.Rule{
		{
			ID:      "order-template",
			Query:   "INSERT INTO orders (item) VALUES ($1)",
			Columns: []string{"id", "item", "created_at"},
			Rows: [][]string{
				{"{{uuid}}", "{{param 1}}", "{{today}}"},
			},
			Tag: "INSERT 0 1",
		},
	}

	srv, addr := startTestServer(t, rules)
	defer srv.Stop()

	conn := connectAndHandshake(t, addr)
	defer conn.Close()

	// Use extended query to pass param $1 = "Mechanical Keyboard"
	query := "INSERT INTO orders (item) VALUES ($1)"
	pName := "p_order"
	var pPayload bytes.Buffer
	pPayload.WriteString(pName + "\x00")
	pPayload.WriteString(query + "\x00")
	_ = binary.Write(&pPayload, binary.BigEndian, int16(0))

	var pPacket bytes.Buffer
	pPacket.WriteByte('P')
	_ = binary.Write(&pPacket, binary.BigEndian, uint32(4+pPayload.Len()))
	pPacket.Write(pPayload.Bytes())
	_, _ = conn.Write(pPacket.Bytes())

	var tBuf [1]byte
	var lBuf [4]byte
	_, _ = io.ReadFull(conn, tBuf[:])
	_, _ = io.ReadFull(conn, lBuf[:])

	// Bind
	portalName := "portal_order"
	var bPayload bytes.Buffer
	bPayload.WriteString(portalName + "\x00")
	bPayload.WriteString(pName + "\x00")
	_ = binary.Write(&bPayload, binary.BigEndian, int16(0))
	_ = binary.Write(&bPayload, binary.BigEndian, int16(1))
	val := []byte("Mechanical Keyboard")
	_ = binary.Write(&bPayload, binary.BigEndian, int32(len(val)))
	bPayload.Write(val)
	_ = binary.Write(&bPayload, binary.BigEndian, int16(0))

	var bPacket bytes.Buffer
	bPacket.WriteByte('B')
	_ = binary.Write(&bPacket, binary.BigEndian, uint32(4+bPayload.Len()))
	bPacket.Write(bPayload.Bytes())
	_, _ = conn.Write(bPacket.Bytes())
	_, _ = io.ReadFull(conn, tBuf[:])
	_, _ = io.ReadFull(conn, lBuf[:])

	// Execute + Sync
	var ePacket bytes.Buffer
	ePacket.WriteByte('E')
	_ = binary.Write(&ePacket, binary.BigEndian, uint32(4+len(portalName)+1+4))
	ePacket.WriteString(portalName + "\x00")
	_ = binary.Write(&ePacket, binary.BigEndian, int32(0))
	ePacket.Write([]byte{'S', 0, 0, 0, 4})
	_, _ = conn.Write(ePacket.Bytes())

	var returnedRowValues []string
	for {
		_, err := io.ReadFull(conn, tBuf[:])
		if err != nil {
			break
		}
		_, _ = io.ReadFull(conn, lBuf[:])
		pLen := binary.BigEndian.Uint32(lBuf[:]) - 4
		p := make([]byte, pLen)
		_, _ = io.ReadFull(conn, p)

		if tBuf[0] == 'D' {
			// Read DataRow columns
			colCount := binary.BigEndian.Uint16(p[0:2])
			idx := 2
			for c := 0; c < int(colCount); c++ {
				colLen := int32(binary.BigEndian.Uint32(p[idx : idx+4]))
				idx += 4
				if colLen > 0 {
					returnedRowValues = append(returnedRowValues, string(p[idx:idx+int(colLen)]))
					idx += int(colLen)
				}
			}
		} else if tBuf[0] == 'Z' {
			break
		}
	}

	if len(returnedRowValues) != 3 {
		t.Fatalf("expected 3 templated columns, got %d: %v", len(returnedRowValues), returnedRowValues)
	}

	// Verify UUID format (36 chars)
	if len(returnedRowValues[0]) != 36 {
		t.Errorf("expected 36-char UUID, got %s", returnedRowValues[0])
	}
	// Verify param 1
	if returnedRowValues[1] != "Mechanical Keyboard" {
		t.Errorf("expected 'Mechanical Keyboard', got %s", returnedRowValues[1])
	}
	// Verify today's date
	expectedToday := time.Now().UTC().Format("2006-01-02")
	if returnedRowValues[2] != expectedToday {
		t.Errorf("expected date %s, got %s", expectedToday, returnedRowValues[2])
	}
}

func TestEndToEndStatefulCRUD(t *testing.T) {
	eng := mock.NewEngine(nil)
	eng.EnableStateful(true)
	srv := server.NewServer("127.0.0.1:0", eng, false)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer srv.Stop()

	conn := connectAndHandshake(t, srv.Addr())
	defer conn.Close()

	runSimpleQuery := func(sql string) (string, []string) {
		var qBuf bytes.Buffer
		qBuf.WriteByte('Q')
		_ = binary.Write(&qBuf, binary.BigEndian, uint32(4+len(sql)+1))
		qBuf.WriteString(sql)
		qBuf.WriteByte(0)
		_, _ = conn.Write(qBuf.Bytes())

		tag := ""
		var rows []string
		for {
			var tBuf [1]byte
			var lBuf [4]byte
			if _, err := io.ReadFull(conn, tBuf[:]); err != nil {
				break
			}
			_, _ = io.ReadFull(conn, lBuf[:])
			pLen := binary.BigEndian.Uint32(lBuf[:]) - 4
			p := make([]byte, pLen)
			_, _ = io.ReadFull(conn, p)

			if tBuf[0] == 'C' {
				tag = string(p[:len(p)-1])
			} else if tBuf[0] == 'D' {
				colCount := binary.BigEndian.Uint16(p[0:2])
				idx := 2
				for c := 0; c < int(colCount); c++ {
					colLen := int32(binary.BigEndian.Uint32(p[idx : idx+4]))
					idx += 4
					if colLen > 0 {
						rows = append(rows, string(p[idx:idx+int(colLen)]))
						idx += int(colLen)
					}
				}
			} else if tBuf[0] == 'Z' {
				break
			}
		}
		return tag, rows
	}

	// 1. Create table
	tag, _ := runSimpleQuery("CREATE TABLE items (id int, name text);")
	if tag != "CREATE TABLE" {
		t.Errorf("expected CREATE TABLE tag, got %q", tag)
	}

	// 2. Insert items
	tag, _ = runSimpleQuery("INSERT INTO items (id, name) VALUES (1, 'Book');")
	if tag != "INSERT 0 1" {
		t.Errorf("expected INSERT 0 1 tag, got %q", tag)
	}

	// 3. Query item
	tag, rows := runSimpleQuery("SELECT name FROM items WHERE id = 1;")
	if tag != "SELECT 1" || len(rows) != 1 || rows[0] != "Book" {
		t.Errorf("unexpected query result: tag=%s, rows=%v", tag, rows)
	}
}

func TestEndToEndCatalogIntrospection(t *testing.T) {
	srv, addr := startTestServer(t, nil)
	defer srv.Stop()

	conn := connectAndHandshake(t, addr)
	defer conn.Close()

	runSimpleQuery := func(sql string) []string {
		var qBuf bytes.Buffer
		qBuf.WriteByte('Q')
		_ = binary.Write(&qBuf, binary.BigEndian, uint32(4+len(sql)+1))
		qBuf.WriteString(sql)
		qBuf.WriteByte(0)
		_, _ = conn.Write(qBuf.Bytes())

		var rows []string
		for {
			var tBuf [1]byte
			var lBuf [4]byte
			if _, err := io.ReadFull(conn, tBuf[:]); err != nil {
				break
			}
			_, _ = io.ReadFull(conn, lBuf[:])
			pLen := binary.BigEndian.Uint32(lBuf[:]) - 4
			p := make([]byte, pLen)
			_, _ = io.ReadFull(conn, p)

			if tBuf[0] == 'D' {
				colCount := binary.BigEndian.Uint16(p[0:2])
				idx := 2
				for c := 0; c < int(colCount); c++ {
					colLen := int32(binary.BigEndian.Uint32(p[idx : idx+4]))
					idx += 4
					if colLen > 0 {
						rows = append(rows, string(p[idx:idx+int(colLen)]))
						idx += int(colLen)
					}
				}
			} else if tBuf[0] == 'Z' {
				break
			}
		}
		return rows
	}

	// Check current_schema()
	rows := runSimpleQuery("SELECT current_schema();")
	if len(rows) != 1 || rows[0] != "public" {
		t.Errorf("expected 'public' schema, got %v", rows)
	}

	// Check version()
	rows = runSimpleQuery("SELECT version();")
	if len(rows) != 1 || !strings.Contains(rows[0], "PostgreSQL 16.2") {
		t.Errorf("expected PostgreSQL 16.2 version string, got %v", rows)
	}
}
