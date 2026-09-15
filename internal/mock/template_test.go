package mock

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRenderTemplate(t *testing.T) {
	ResetSequence()

	// 1. Static text unmodified
	if got := RenderTemplate("hello world", nil); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}

	// 2. UUID
	uuidStr := RenderTemplate("id-{{uuid}}", nil)
	if !strings.HasPrefix(uuidStr, "id-") || len(uuidStr) != 39 {
		t.Errorf("unexpected uuid format: %s", uuidStr)
	}

	// 3. Sequence
	s1 := RenderTemplate("item-{{sequence}}", nil)
	s2 := RenderTemplate("item-{{seq}}", nil)
	if s1 != "item-1" || s2 != "item-2" {
		t.Errorf("unexpected sequence values: %s, %s", s1, s2)
	}

	// 4. Parameter substitution
	params := [][]byte{[]byte("alice"), []byte("42")}
	p1 := RenderTemplate("user: {{param 1}}, age: {{param $2}}", params)
	if p1 != "user: alice, age: 42" {
		t.Errorf("unexpected param output: %s", p1)
	}

	// Out of bounds param
	pNull := RenderTemplate("val: {{param 99}}", params)
	if pNull != "val: NULL" {
		t.Errorf("expected NULL for out-of-bounds param, got %s", pNull)
	}

	// 5. Dates and timestamps
	today := RenderTemplate("{{today}}", nil)
	expectedToday := time.Now().UTC().Format("2006-01-02")
	if today != expectedToday {
		t.Errorf("expected today %s, got %s", expectedToday, today)
	}

	nowStr := RenderTemplate("{{now}}", nil)
	if len(nowStr) < 19 {
		t.Errorf("unexpected now output: %s", nowStr)
	}

	// 6. Random integer
	rndStr := RenderTemplate("{{random 10 20}}", nil)
	n, err := strconv.Atoi(rndStr)
	if err != nil || n < 10 || n > 20 {
		t.Errorf("unexpected random int: %s", rndStr)
	}

	// 7. Email
	email := RenderTemplate("{{email}}", nil)
	if !strings.HasPrefix(email, "user-") || !strings.HasSuffix(email, "@example.com") {
		t.Errorf("unexpected email format: %s", email)
	}
}
