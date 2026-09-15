package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
)

// WriteSSLRefusal sends the single byte 'N' refusing SSL negotiation
func WriteSSLRefusal(w io.Writer) error {
	_, err := w.Write([]byte{'N'})
	return err
}

// WriteAuthOK sends AuthenticationOk ('R')
func WriteAuthOK(w io.Writer) error {
	buf := make([]byte, 9)
	buf[0] = MsgTypeAuth
	binary.BigEndian.PutUint32(buf[1:5], 8)
	binary.BigEndian.PutUint32(buf[5:9], 0) // 0 = AuthOK
	_, err := w.Write(buf)
	return err
}

// WriteParameterStatus sends ParameterStatus ('S')
func WriteParameterStatus(w io.Writer, name, value string) error {
	payloadLen := 4 + len(name) + 1 + len(value) + 1
	buf := make([]byte, 1+payloadLen)
	buf[0] = MsgTypeParameterStatus
	binary.BigEndian.PutUint32(buf[1:5], uint32(payloadLen))

	idx := 5
	copy(buf[idx:], name)
	idx += len(name)
	buf[idx] = 0
	idx++

	copy(buf[idx:], value)
	idx += len(value)
	buf[idx] = 0

	_, err := w.Write(buf)
	return err
}

// StandardParameters holds the default parameters emitted on startup
var StandardParameters = map[string]string{
	"server_version":              "16.2 (PGWire-Mock)",
	"server_encoding":             "UTF8",
	"client_encoding":             "UTF8",
	"application_name":            "pgwire-mock",
	"is_superuser":                "on",
	"session_authorization":       "postgres",
	"standard_conforming_strings": "on",
	"DateStyle":                   "ISO, MDY",
	"IntervalStyle":               "postgres",
	"TimeZone":                    "UTC",
	"integer_datetimes":           "on",
}

// WriteStandardParameterStatuses emits the standard baseline parameters
func WriteStandardParameterStatuses(w io.Writer) error {
	for k, v := range StandardParameters {
		if err := WriteParameterStatus(w, k, v); err != nil {
			return err
		}
	}
	return nil
}

// WriteBackendKeyData sends process ID and cancellation secret key ('K')
func WriteBackendKeyData(w io.Writer, pid, secretKey int32) error {
	buf := make([]byte, 13)
	buf[0] = MsgTypeBackendKeyData
	binary.BigEndian.PutUint32(buf[1:5], 12)
	binary.BigEndian.PutUint32(buf[5:9], uint32(pid))
	binary.BigEndian.PutUint32(buf[9:13], uint32(secretKey))
	_, err := w.Write(buf)
	return err
}

// WriteReadyForQuery sends ReadyForQuery ('Z') with transaction status indicator
func WriteReadyForQuery(w io.Writer, txStatus byte) error {
	buf := []byte{MsgTypeReadyForQuery, 0, 0, 0, 5, txStatus}
	_, err := w.Write(buf)
	return err
}

// WriteRowDescription sends RowDescription ('T')
func WriteRowDescription(w io.Writer, fields []FieldDescription) error {
	var body bytes.Buffer
	numFields := int16(len(fields))
	if err := binary.Write(&body, binary.BigEndian, numFields); err != nil {
		return err
	}

	for _, f := range fields {
		body.WriteString(f.Name)
		body.WriteByte(0)
		_ = binary.Write(&body, binary.BigEndian, f.TableOID)
		_ = binary.Write(&body, binary.BigEndian, f.ColumnAttr)
		_ = binary.Write(&body, binary.BigEndian, f.DataTypeOID)
		_ = binary.Write(&body, binary.BigEndian, f.DataTypeSize)
		_ = binary.Write(&body, binary.BigEndian, f.TypeModifier)
		_ = binary.Write(&body, binary.BigEndian, f.FormatCode)
	}

	packetLen := uint32(4 + body.Len())
	var header [5]byte
	header[0] = MsgTypeRowDescription
	binary.BigEndian.PutUint32(header[1:5], packetLen)

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

// WriteDataRow sends DataRow ('D')
func WriteDataRow(w io.Writer, values [][]byte) error {
	var body bytes.Buffer
	numCols := int16(len(values))
	if err := binary.Write(&body, binary.BigEndian, numCols); err != nil {
		return err
	}

	for _, val := range values {
		if val == nil {
			_ = binary.Write(&body, binary.BigEndian, int32(-1)) // SQL NULL
		} else {
			_ = binary.Write(&body, binary.BigEndian, int32(len(val)))
			body.Write(val)
		}
	}

	packetLen := uint32(4 + body.Len())
	var header [5]byte
	header[0] = MsgTypeDataRow
	binary.BigEndian.PutUint32(header[1:5], packetLen)

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

// WriteCommandComplete sends CommandComplete ('C')
func WriteCommandComplete(w io.Writer, tag string) error {
	payloadLen := 4 + len(tag) + 1
	buf := make([]byte, 1+payloadLen)
	buf[0] = MsgTypeCommandComplete
	binary.BigEndian.PutUint32(buf[1:5], uint32(payloadLen))
	copy(buf[5:], tag)
	buf[len(buf)-1] = 0

	_, err := w.Write(buf)
	return err
}

// WriteEmptyQueryResponse sends EmptyQueryResponse ('I')
func WriteEmptyQueryResponse(w io.Writer) error {
	buf := []byte{MsgTypeEmptyQueryResponse, 0, 0, 0, 4}
	_, err := w.Write(buf)
	return err
}

// WriteErrorResponse sends ErrorResponse ('E')
func WriteErrorResponse(w io.Writer, resp ErrorResponse) error {
	var body bytes.Buffer

	severity := resp.Severity
	if severity == "" {
		severity = "ERROR"
	}
	code := resp.Code
	if code == "" {
		code = "XX000" // Internal error
	}

	body.WriteByte(ErrorSeverity)
	body.WriteString(severity)
	body.WriteByte(0)

	body.WriteByte(ErrorSeverityNonLocalized)
	body.WriteString(severity)
	body.WriteByte(0)

	body.WriteByte(ErrorCode)
	body.WriteString(code)
	body.WriteByte(0)

	body.WriteByte(ErrorMessage)
	body.WriteString(resp.Message)
	body.WriteByte(0)

	if resp.Detail != "" {
		body.WriteByte(ErrorDetail)
		body.WriteString(resp.Detail)
		body.WriteByte(0)
	}

	if resp.Hint != "" {
		body.WriteByte(ErrorHint)
		body.WriteString(resp.Hint)
		body.WriteByte(0)
	}

	body.WriteByte(0) // Final null terminator

	packetLen := uint32(4 + body.Len())
	var header [5]byte
	header[0] = MsgTypeErrorResponse
	binary.BigEndian.PutUint32(header[1:5], packetLen)

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

// WriteParseComplete sends ParseComplete ('1')
func WriteParseComplete(w io.Writer) error {
	buf := []byte{MsgTypeParseComplete, 0, 0, 0, 4}
	_, err := w.Write(buf)
	return err
}

// WriteBindComplete sends BindComplete ('2')
func WriteBindComplete(w io.Writer) error {
	buf := []byte{MsgTypeBindComplete, 0, 0, 0, 4}
	_, err := w.Write(buf)
	return err
}

// WriteCloseComplete sends CloseComplete ('3')
func WriteCloseComplete(w io.Writer) error {
	buf := []byte{MsgTypeCloseComplete, 0, 0, 0, 4}
	_, err := w.Write(buf)
	return err
}

// WriteNoData sends NoData ('n')
func WriteNoData(w io.Writer) error {
	buf := []byte{MsgTypeNoData, 0, 0, 0, 4}
	_, err := w.Write(buf)
	return err
}

// WriteParameterDescription sends ParameterDescription ('t')
func WriteParameterDescription(w io.Writer, paramOIDs []int32) error {
	var body bytes.Buffer
	_ = binary.Write(&body, binary.BigEndian, int16(len(paramOIDs)))
	for _, oid := range paramOIDs {
		_ = binary.Write(&body, binary.BigEndian, oid)
	}

	packetLen := uint32(4 + body.Len())
	var header [5]byte
	header[0] = MsgTypeParameterDescription
	binary.BigEndian.PutUint32(header[1:5], packetLen)

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}
