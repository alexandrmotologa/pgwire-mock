package mock

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var globalSequence int64

var templatePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// GenerateUUIDv4 generates a random RFC 4122 v4 UUID string
func GenerateUUIDv4() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40 // Version 4
	u[8] = (u[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// NextSequence atomically increments and returns a global integer
func NextSequence() int64 {
	return atomic.AddInt64(&globalSequence, 1)
}

// ResetSequence resets the global generator counter
func ResetSequence() {
	atomic.StoreInt64(&globalSequence, 0)
}

// RenderTemplate parses and substitutes dynamic tags in a cell template string.
// Supported expressions:
//
//	{{param 1}} or {{param $1}}
//	{{uuid}}
//	{{now}}
//	{{now + 1h}}, {{now - 30m}}, {{now + 2d}}
//	{{today}}
//	{{today + 1d}}, {{today - 7d}}
//	{{sequence}} or {{seq}}
//	{{random min max}} or {{random_int min max}}
//	{{email}}
func RenderTemplate(val string, params [][]byte) string {
	if !strings.Contains(val, "{{") {
		return val
	}

	return templatePattern.ReplaceAllStringFunc(val, func(match string) string {
		sub := templatePattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}

		expr := strings.TrimSpace(sub[1])
		parts := strings.Fields(expr)
		if len(parts) == 0 {
			return match
		}

		op := strings.ToLower(parts[0])

		switch op {
		case "param":
			if len(parts) < 2 {
				return ""
			}
			idxStr := strings.TrimPrefix(parts[1], "$")
			idx, err := strconv.Atoi(idxStr)
			if err != nil || idx < 1 || idx > len(params) {
				return "NULL"
			}
			p := params[idx-1]
			if p == nil {
				return "NULL"
			}
			return string(p)

		case "uuid":
			return GenerateUUIDv4()

		case "now":
			now := time.Now().UTC()
			if len(parts) >= 3 {
				offset := parseDurationOffset(parts[1], parts[2])
				now = now.Add(offset)
			}
			return now.Format("2006-01-02 15:04:05.000000+00")

		case "today":
			now := time.Now().UTC()
			if len(parts) >= 3 {
				offset := parseDurationOffset(parts[1], parts[2])
				now = now.Add(offset)
			}
			return now.Format("2006-01-02")

		case "sequence", "seq":
			return strconv.FormatInt(NextSequence(), 10)

		case "random", "random_int":
			if len(parts) >= 3 {
				minVal, err1 := strconv.ParseInt(parts[1], 10, 64)
				maxVal, err2 := strconv.ParseInt(parts[2], 10, 64)
				if err1 == nil && err2 == nil && maxVal >= minVal {
					span := maxVal - minVal + 1
					n, err := rand.Int(rand.Reader, big.NewInt(span))
					if err == nil {
						return strconv.FormatInt(minVal+n.Int64(), 10)
					}
				}
			}
			return "0"

		case "email":
			n, _ := rand.Int(rand.Reader, big.NewInt(900000))
			return fmt.Sprintf("user-%d@example.com", 100000+n.Int64())

		default:
			return match
		}
	})
}

func parseDurationOffset(sign, durationStr string) time.Duration {
	multiplier := time.Duration(1)
	if sign == "-" {
		multiplier = -1
	}

	// Support day offsets like '1d', '7d'
	if strings.HasSuffix(durationStr, "d") {
		daysStr := strings.TrimSuffix(durationStr, "d")
		days, err := strconv.Atoi(daysStr)
		if err == nil {
			return multiplier * time.Duration(days) * 24 * time.Hour
		}
	}

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0
	}
	return multiplier * d
}
