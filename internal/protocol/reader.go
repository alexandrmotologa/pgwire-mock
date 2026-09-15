package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	ErrUnexpectedEOF     = errors.New("unexpected EOF reading pgwire packet")
	ErrInvalidPacketSize = errors.New("invalid pgwire packet size")
	ErrUnsupportedProto  = errors.New("unsupported protocol version")
)

// ReadStartupOrSSL reads the initial handshake packet from the client.
// If the client sends an SSLRequest, isSSL is true and startup is nil.
// If the client sends a StartupMessage, isSSL is false and startup is populated.
func ReadStartupOrSSL(r io.Reader) (isSSL bool, startup *StartupMessage, err error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return false, nil, err
	}
	packetLen := binary.BigEndian.Uint32(lenBuf[:])
	if packetLen < 8 || packetLen > 10000 {
		return false, nil, fmt.Errorf("%w: packetLen=%d", ErrInvalidPacketSize, packetLen)
	}

	var codeBuf [4]byte
	if _, err := io.ReadFull(r, codeBuf[:]); err != nil {
		return false, nil, err
	}
	code := binary.BigEndian.Uint32(codeBuf[:])

	if code == SSLRequestCode {
		return true, nil, nil
	}

	if code != ProtocolVersion3 {
		return false, nil, fmt.Errorf("%w: code=%d", ErrUnsupportedProto, code)
	}

	remaining := packetLen - 8
	payload := make([]byte, remaining)
	if _, err := io.ReadFull(r, payload); err != nil {
		return false, nil, err
	}

	params := make(map[string]string)
	buf := payload
	for len(buf) > 0 {
		nullIdx := bytes.IndexByte(buf, 0)
		if nullIdx == -1 || nullIdx == 0 {
			// Zero byte marks end of parameters
			break
		}
		key := string(buf[:nullIdx])
		buf = buf[nullIdx+1:]

		nullIdxVal := bytes.IndexByte(buf, 0)
		if nullIdxVal == -1 {
			break
		}
		val := string(buf[:nullIdxVal])
		buf = buf[nullIdxVal+1:]

		params[key] = val
	}

	return false, &StartupMessage{
		ProtocolVersion: int32(code),
		Parameters:      params,
	}, nil
}

// ReadMessageHeader reads a standard 1-byte message type and 4-byte payload length.
// The returned length represents the remaining payload length (packet length minus 4).
func ReadMessageHeader(r io.Reader) (msgType byte, payloadLength int32, err error) {
	var typeBuf [1]byte
	if _, err := io.ReadFull(r, typeBuf[:]); err != nil {
		return 0, 0, err
	}
	msgType = typeBuf[0]

	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return 0, 0, err
	}
	rawLen := binary.BigEndian.Uint32(lenBuf[:])
	if rawLen < 4 {
		return 0, 0, ErrInvalidPacketSize
	}

	return msgType, int32(rawLen - 4), nil
}

// ReadQuery parses Simple Query payload
func ReadQuery(payload []byte) (*QueryMessage, error) {
	nullIdx := bytes.IndexByte(payload, 0)
	if nullIdx == -1 {
		return &QueryMessage{Query: string(payload)}, nil
	}
	return &QueryMessage{Query: string(payload[:nullIdx])}, nil
}

// ReadParse parses Extended Query Parse payload
func ReadParse(payload []byte) (*ParseMessage, error) {
	buf := bytes.NewReader(payload)
	stmtName, err := readNullString(buf)
	if err != nil {
		return nil, err
	}
	query, err := readNullString(buf)
	if err != nil {
		return nil, err
	}

	var numParams int16
	if err := binary.Read(buf, binary.BigEndian, &numParams); err != nil {
		if errors.Is(err, io.EOF) {
			return &ParseMessage{StatementName: stmtName, Query: query}, nil
		}
		return nil, err
	}

	var paramOIDs []int32
	if numParams > 0 {
		paramOIDs = make([]int32, numParams)
		for i := 0; i < int(numParams); i++ {
			if err := binary.Read(buf, binary.BigEndian, &paramOIDs[i]); err != nil {
				return nil, err
			}
		}
	}

	return &ParseMessage{
		StatementName: stmtName,
		Query:         query,
		ParamOIDs:     paramOIDs,
	}, nil
}

// ReadBind parses Extended Query Bind payload
func ReadBind(payload []byte) (*BindMessage, error) {
	buf := bytes.NewReader(payload)
	portalName, err := readNullString(buf)
	if err != nil {
		return nil, err
	}
	stmtName, err := readNullString(buf)
	if err != nil {
		return nil, err
	}

	var numFormats int16
	if err := binary.Read(buf, binary.BigEndian, &numFormats); err != nil {
		return nil, err
	}
	paramFormats := make([]int16, numFormats)
	for i := 0; i < int(numFormats); i++ {
		if err := binary.Read(buf, binary.BigEndian, &paramFormats[i]); err != nil {
			return nil, err
		}
	}

	var numParams int16
	if err := binary.Read(buf, binary.BigEndian, &numParams); err != nil {
		return nil, err
	}

	params := make([][]byte, numParams)
	for i := 0; i < int(numParams); i++ {
		var paramLen int32
		if err := binary.Read(buf, binary.BigEndian, &paramLen); err != nil {
			return nil, err
		}
		if paramLen == -1 {
			params[i] = nil // SQL NULL
		} else if paramLen >= 0 {
			val := make([]byte, paramLen)
			if _, err := io.ReadFull(buf, val); err != nil {
				return nil, err
			}
			params[i] = val
		}
	}

	var numResultFormats int16
	var resultFormats []int16
	if err := binary.Read(buf, binary.BigEndian, &numResultFormats); err == nil && numResultFormats > 0 {
		resultFormats = make([]int16, numResultFormats)
		for i := 0; i < int(numResultFormats); i++ {
			if err := binary.Read(buf, binary.BigEndian, &resultFormats[i]); err != nil {
				return nil, err
			}
		}
	}

	return &BindMessage{
		PortalName:        portalName,
		StatementName:     stmtName,
		ParamFormats:      paramFormats,
		Parameters:        params,
		ResultFormatCodes: resultFormats,
	}, nil
}

// ReadDescribe parses Extended Query Describe payload
func ReadDescribe(payload []byte) (*DescribeMessage, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty describe payload")
	}
	targetType := payload[0]
	buf := bytes.NewReader(payload[1:])
	name, err := readNullString(buf)
	if err != nil {
		return nil, err
	}
	return &DescribeMessage{
		TargetType: targetType,
		Name:       name,
	}, nil
}

// ReadExecute parses Extended Query Execute payload
func ReadExecute(payload []byte) (*ExecuteMessage, error) {
	buf := bytes.NewReader(payload)
	portalName, err := readNullString(buf)
	if err != nil {
		return nil, err
	}
	var maxRows int32
	if err := binary.Read(buf, binary.BigEndian, &maxRows); err != nil {
		if errors.Is(err, io.EOF) {
			maxRows = 0
		} else {
			return nil, err
		}
	}
	return &ExecuteMessage{
		PortalName: portalName,
		MaxRows:    maxRows,
	}, nil
}

// ReadClose parses Extended Query Close payload
func ReadClose(payload []byte) (*CloseMessage, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty close payload")
	}
	targetType := payload[0]
	buf := bytes.NewReader(payload[1:])
	name, err := readNullString(buf)
	if err != nil {
		return nil, err
	}
	return &CloseMessage{
		TargetType: targetType,
		Name:       name,
	}, nil
}

func readNullString(r io.ByteReader) (string, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf), nil
}
