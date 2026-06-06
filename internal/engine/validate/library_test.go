package validate_test

import (
	"testing"

	"mitm_transformation/internal/engine/validate"
)

func TestNotNull(t *testing.T) {
	valid, err := validate.NotNull("hello", nil)
	if !valid || err != nil {
		t.Errorf("expected true, got %v with error: %v", valid, err)
	}

	valid, err = validate.NotNull("", nil)
	if valid || err == nil {
		t.Errorf("expected false, got %v", valid)
	}
}

func TestRegexMatch(t *testing.T) {
	params := map[string]interface{}{
		"pattern": "^[0-9]+$",
	}
	valid, err := validate.RegexMatch("12345", params)
	if !valid || err != nil {
		t.Errorf("expected true, got %v with error: %v", valid, err)
	}

	valid, err = validate.RegexMatch("123a", params)
	if valid || err == nil {
		t.Errorf("expected false for '123a', got %v", valid)
	}
}

func TestRangeCheck(t *testing.T) {
	params := map[string]interface{}{
		"min": 10.0,
		"max": 100.0,
	}
	valid, err := validate.RangeCheck(50, params)
	if !valid || err != nil {
		t.Errorf("expected true, got %v with error: %v", valid, err)
	}

	valid, err = validate.RangeCheck(5, params)
	if valid || err == nil {
		t.Errorf("expected false for 5, got %v", valid)
	}
}

func TestEmail(t *testing.T) {
	valid, err := validate.Email("test@example.com", nil)
	if !valid || err != nil {
		t.Errorf("expected true, got %v with error: %v", valid, err)
	}

	valid, err = validate.Email("invalid-email", nil)
	if valid || err == nil {
		t.Errorf("expected false for 'invalid-email', got %v", valid)
	}
}

func TestInList(t *testing.T) {
	params := map[string]interface{}{
		"allowed": []interface{}{"active", "suspended"},
	}
	valid, err := validate.InList("active", params)
	if !valid || err != nil {
		t.Errorf("expected true, got %v with error: %v", valid, err)
	}

	valid, err = validate.InList("terminated", params)
	if valid || err == nil {
		t.Errorf("expected false for 'terminated', got %v", valid)
	}
}
