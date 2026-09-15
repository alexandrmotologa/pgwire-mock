package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
)

// Postgres epoch starts at 2000-01-01 00:00:00 UTC
var pgEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// EncodeBinaryValue converts a string representation of a value to PostgreSQL binary format
// based on its data type OID. If parsing fails, it falls back to raw UTF-8 bytes.
func EncodeBinaryValue(oid int32, textVal string) []byte {
	switch oid {
	case OIDBool:
		lower := strings.ToLower(strings.TrimSpace(textVal))
		if lower == "true" || lower == "t" || lower == "1" || lower == "yes" {
			return []byte{1}
		}
		return []byte{0}

	case OIDInt2:
		n, err := strconv.ParseInt(strings.TrimSpace(textVal), 10, 16)
		if err != nil {
			return []byte(textVal)
		}
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(n))
		return buf

	case OIDInt4:
		n, err := strconv.ParseInt(strings.TrimSpace(textVal), 10, 32)
		if err != nil {
			return []byte(textVal)
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(n))
		return buf

	case OIDInt8:
		n, err := strconv.ParseInt(strings.TrimSpace(textVal), 10, 64)
		if err != nil {
			return []byte(textVal)
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(n))
		return buf

	case OIDFloat4:
		f, err := strconv.ParseFloat(strings.TrimSpace(textVal), 32)
		if err != nil {
			return []byte(textVal)
		}
		bits := math.Float32bits(float32(f))
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, bits)
		return buf

	case OIDFloat8:
		f, err := strconv.ParseFloat(strings.TrimSpace(textVal), 64)
		if err != nil {
			return []byte(textVal)
		}
		bits := math.Float64bits(f)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, bits)
		return buf

	case OIDBytea:
		trimmed := strings.TrimSpace(textVal)
		if strings.HasPrefix(trimmed, `\x`) {
			decoded, err := hex.DecodeString(trimmed[2:])
			if err == nil {
				return decoded
			}
		}
		return []byte(textVal)

	case OIDDate:
		// int32 days since 2000-01-01
		t, err := time.Parse("2006-01-02", strings.TrimSpace(textVal))
		if err == nil {
			days := int32(t.Sub(pgEpoch).Hours() / 24)
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, uint32(days))
			return buf
		}
		return []byte(textVal)

	case OIDTimestamp, OIDTimestamptz:
		// int64 microseconds since 2000-01-01 00:00:00 UTC
		trimmed := strings.TrimSpace(textVal)
		var parsed time.Time
		var err error
		layouts := []string{
			"2006-01-02 15:04:05.999999-07",
			"2006-01-02 15:04:05.999999+00",
			"2006-01-02 15:04:05.999999",
			"2006-01-02 15:04:05",
			time.RFC3339,
		}
		for _, layout := range layouts {
			parsed, err = time.Parse(layout, trimmed)
			if err == nil {
				break
			}
		}
		if err == nil {
			micros := parsed.Sub(pgEpoch).Microseconds()
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, uint64(micros))
			return buf
		}
		return []byte(textVal)

	default:
		// Text, Varchar, JSON, UUID, etc.
		return []byte(textVal)
	}
}
