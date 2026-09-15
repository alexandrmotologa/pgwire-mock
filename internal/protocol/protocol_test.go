package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSSLRequestHandling(t *testing.T) {
	// Build simulated SSLRequest packet: length 8, code 80877103
	var in bytes.Buffer
	_ = binary.Write(&in, binary.BigEndian, uint32(8))
	_ = binary.Write(&in, binary.BigEndian, uint32(SSLRequestCode))

	isSSL, startup, err := ReadStartupOrSSL(&in)
	if err != nil {
		t.Fatalf("unexpected error reading SSLRequest: %v", err)
	}
	if !isSSL {
		t.Fatalf("expected isSSL=true, got false")
	}
	if startup != nil {
		t.Fatalf("expected nil startup for SSLRequest, got %v", startup)
	}

	var out bytes.Buffer
	if err := WriteSSLRefusal(&out); err != nil {
		t.Fatalf("failed to write SSL refusal: %v", err)
	}
	if out.Len() != 1 || out.Bytes()[0] != 'N' {
		t.Fatalf("expected single byte 'N', got %v", out.Bytes())
	}
}

func TestStartupMessageParsing(t *testing.T) {
	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, uint32(ProtocolVersion3))
	body.WriteString("user\x00postgres\x00database\x00testdb\x00client_encoding\x00UTF8\x00\x00")

	var in bytes.Buffer
	_ = binary.Write(&in, binary.BigEndian, uint32(4+body.Len()))
	in.Write(body.Bytes())

	isSSL, startup, err := ReadStartupOrSSL(&in)
	if err != nil {
		t.Fatalf("failed to parse startup message: %v", err)
	}
	if isSSL {
		t.Fatalf("expected isSSL=false")
	}
	if startup.User() != "postgres" {
		t.Errorf("expected user 'postgres', got %q", startup.User())
	}
	if startup.Database() != "testdb" {
		t.Errorf("expected database 'testdb', got %q", startup.Database())
	}
}

func TestRowDescriptionAndDataRowSerialization(t *testing.T) {
	fields := []FieldDescription{
		{
			Name:         "id",
			TableOID:     16384,
			ColumnAttr:   1,
			DataTypeOID:  OIDInt4,
			DataTypeSize: 4,
			TypeModifier: -1,
			FormatCode:   FormatText,
		},
		{
			Name:         "name",
			TableOID:     16384,
			ColumnAttr:   2,
			DataTypeOID:  OIDVarchar,
			DataTypeSize: -1,
			TypeModifier: -1,
			FormatCode:   FormatText,
		},
	}

	var out bytes.Buffer
	if err := WriteRowDescription(&out, fields); err != nil {
		t.Fatalf("WriteRowDescription failed: %v", err)
	}

	raw := out.Bytes()
	if raw[0] != MsgTypeRowDescription {
		t.Fatalf("expected msgType 'T', got %c", raw[0])
	}

	packetLen := binary.BigEndian.Uint32(raw[1:5])
	if int(packetLen)+1 != len(raw) {
		t.Fatalf("packet length mismatch: header specifies %d, actual total %d", packetLen, len(raw))
	}

	// Test DataRow serialization including NULL
	out.Reset()
	rowValues := [][]byte{
		[]byte("42"),
		nil, // NULL
	}
	if err := WriteDataRow(&out, rowValues); err != nil {
		t.Fatalf("WriteDataRow failed: %v", err)
	}

	rawRow := out.Bytes()
	if rawRow[0] != MsgTypeDataRow {
		t.Fatalf("expected msgType 'D', got %c", rawRow[0])
	}
}

func TestErrorResponseSerialization(t *testing.T) {
	var out bytes.Buffer
	errResp := ErrorResponse{
		Severity: "ERROR",
		Code:     "42P01",
		Message:  "relation \"orders\" does not exist",
		Detail:   "Table was not defined in mock rules.",
		Hint:     "Add a rule in your YAML mock configuration.",
	}

	if err := WriteErrorResponse(&out, errResp); err != nil {
		t.Fatalf("WriteErrorResponse failed: %v", err)
	}

	raw := out.Bytes()
	if raw[0] != MsgTypeErrorResponse {
		t.Fatalf("expected msgType 'E', got %c", raw[0])
	}
}

func TestSimpleQueryParsing(t *testing.T) {
	queryPayload := []byte("SELECT 1;\x00")
	qMsg, err := ReadQuery(queryPayload)
	if err != nil {
		t.Fatalf("ReadQuery failed: %v", err)
	}
	if qMsg.Query != "SELECT 1;" {
		t.Errorf("expected 'SELECT 1;', got %q", qMsg.Query)
	}
}

func TestCommandCompleteAndReadyForQuery(t *testing.T) {
	var out bytes.Buffer
	if err := WriteCommandComplete(&out, "SELECT 10"); err != nil {
		t.Fatalf("WriteCommandComplete failed: %v", err)
	}
	if err := WriteReadyForQuery(&out, TxStatusIdle); err != nil {
		t.Fatalf("WriteReadyForQuery failed: %v", err)
	}

	raw := out.Bytes()
	if raw[0] != MsgTypeCommandComplete {
		t.Fatalf("expected 'C', got %c", raw[0])
	}

	// Last 6 bytes should be ReadyForQuery ('Z', 0, 0, 0, 5, 'I')
	zPart := raw[len(raw)-6:]
	if zPart[0] != MsgTypeReadyForQuery || zPart[5] != TxStatusIdle {
		t.Fatalf("invalid ReadyForQuery packet: %v", zPart)
	}
}
