package transform_test

import (
	"testing"
	"time"

	"mitm_transformation/internal/engine/transform"
)

func TestTrimWhitespace(t *testing.T) {
	val, err := transform.TrimWhitespace("  hello  ", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got '%v'", val)
	}
}

func TestToUpper(t *testing.T) {
	val, err := transform.ToUpper("hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "HELLO" {
		t.Errorf("expected 'HELLO', got '%v'", val)
	}
}

func TestDefaultValue(t *testing.T) {
	params := map[string]interface{}{"value": "N/A"}
	val, err := transform.DefaultValue("", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "N/A" {
		t.Errorf("expected 'N/A', got '%v'", val)
	}
}

func TestRegexReplace(t *testing.T) {
	params := map[string]interface{}{
		"pattern": "[^0-9]",
		"replace": "",
	}
	val, err := transform.RegexReplace("abc123def", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "123" {
		t.Errorf("expected '123', got '%v'", val)
	}
}

func TestParseDate(t *testing.T) {
	params := map[string]interface{}{
		"input_format":  "2006-01-02",
		"output_format": time.RFC3339,
	}
	val, err := transform.ParseDate("2026-06-06", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "2026-06-06T00:00:00Z" {
		t.Errorf("expected '2026-06-06T00:00:00Z', got '%v'", val)
	}
}

func TestStringSplit(t *testing.T) {
	params := map[string]interface{}{
		"separator": ",",
		"index":     1,
	}
	val, err := transform.StringSplit("a,b,c", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "b" {
		t.Errorf("expected 'b', got '%v'", val)
	}
}

func TestCastType(t *testing.T) {
	params := map[string]interface{}{"target_type": "integer"}
	val, err := transform.CastType("123", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 123 {
		t.Errorf("expected int 123, got '%v'", val)
	}
}
