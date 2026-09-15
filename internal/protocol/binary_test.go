package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestEncodeBinaryValue(t *testing.T) {
	// 1. Bool
	bTrue := EncodeBinaryValue(OIDBool, "true")
	if len(bTrue) != 1 || bTrue[0] != 1 {
		t.Errorf("expected [1] for true, got %v", bTrue)
	}
	bFalse := EncodeBinaryValue(OIDBool, "f")
	if len(bFalse) != 1 || bFalse[0] != 0 {
		t.Errorf("expected [0] for false, got %v", bFalse)
	}

	// 2. Int2
	bInt2 := EncodeBinaryValue(OIDInt2, "42")
	if len(bInt2) != 2 || binary.BigEndian.Uint16(bInt2) != 42 {
		t.Errorf("unexpected int2 encoding: %v", bInt2)
	}

	// 3. Int4
	bInt4 := EncodeBinaryValue(OIDInt4, "123456")
	if len(bInt4) != 4 || binary.BigEndian.Uint32(bInt4) != 123456 {
		t.Errorf("unexpected int4 encoding: %v", bInt4)
	}

	// 4. Int8
	bInt8 := EncodeBinaryValue(OIDInt8, "9876543210")
	if len(bInt8) != 8 || binary.BigEndian.Uint64(bInt8) != 9876543210 {
		t.Errorf("unexpected int8 encoding: %v", bInt8)
	}

	// 5. Float4
	bFloat4 := EncodeBinaryValue(OIDFloat4, "3.14")
	f4 := math.Float32frombits(binary.BigEndian.Uint32(bFloat4))
	if math.Abs(float64(f4-3.14)) > 0.001 {
		t.Errorf("unexpected float4 encoding: %f", f4)
	}

	// 6. Float8
	bFloat8 := EncodeBinaryValue(OIDFloat8, "2.718281828459")
	f8 := math.Float64frombits(binary.BigEndian.Uint64(bFloat8))
	if math.Abs(f8-2.718281828459) > 0.0000001 {
		t.Errorf("unexpected float8 encoding: %f", f8)
	}

	// 7. Bytea (\x deadbeef)
	bBytea := EncodeBinaryValue(OIDBytea, `\xdeadbeef`)
	expectedBytea := []byte{0xde, 0xad, 0xbe, 0xef}
	if !bytes.Equal(bBytea, expectedBytea) {
		t.Errorf("unexpected bytea encoding: %x", bBytea)
	}
}
