package engine_test

import (
	"crypto/rand"
	"encoding/json"
	"testing"

	"mitm_transformation/internal/crypto"
	"mitm_transformation/internal/db"
	"mitm_transformation/internal/engine"
	"mitm_transformation/internal/engine/transform"
	"mitm_transformation/internal/engine/validate"
)

func setupTestEngine() *engine.PipelineEngine {
	registry := engine.NewEngineRegistry()
	transform.RegisterAll(registry)
	validate.RegisterAll(registry)
	return engine.NewPipelineEngine(registry)
}

func TestProcessPayload(t *testing.T) {
	eng := setupTestEngine()

	sourceID := "src-1"
	masterKey := make([]byte, 32)
	rand.Read(masterKey)
	wrappedKey, err := crypto.GenerateWrappedDEK(masterKey)
	if err != nil {
		t.Fatalf("failed to generate wrapped key: %v", err)
	}

	ruleSet := &db.RuleSet{
		TargetFields: map[string]db.MappingTargetField{
			"tf-1": {ID: "tf-1", FieldName: "first_name", Encrypted: false},
			"tf-2": {ID: "tf-2", FieldName: "email", Encrypted: true},
		},
		Rules: []db.MappingRule{
			{
				SourceID:            "src-1",
				TargetFieldID:       "tf-1",
				SourceField:         "FName",
				TransformationChain: json.RawMessage(`[{"name": "trim_whitespace", "parameters": {}}]`),
				ValidationChain:     json.RawMessage(`[{"name": "not_null", "parameters": {}}]`),
				ParsedTransformations: []db.RuleStep{{Name: "trim_whitespace"}},
				ParsedValidations:     []db.RuleStep{{Name: "not_null"}},
			},
			{
				SourceID:            "src-1",
				TargetFieldID:       "tf-2",
				SourceField:         "Mail",
				TransformationChain: json.RawMessage(`[{"name": "to_lower", "parameters": {}}]`),
				ValidationChain:     json.RawMessage(`[{"name": "email", "parameters": {}}]`),
				ParsedTransformations: []db.RuleStep{{Name: "to_lower"}},
				ParsedValidations:     []db.RuleStep{{Name: "email"}},
			},
		},
	}

	payload := map[string]interface{}{
		"FName": "  Robert  ",
		"Mail":  "TEST@EXAMPLE.COM",
	}

	result, errs := eng.ProcessPayload(payload, sourceID, ruleSet, masterKey, wrappedKey)
	if len(errs) > 0 {
		t.Fatalf("unexpected pipeline errors: %v", errs)
	}

	if result["first_name"] != "Robert" {
		t.Errorf("expected first_name 'Robert', got '%v'", result["first_name"])
	}

	// email is encrypted, so we expect a map with ciphertext and nonce
	emailResult, ok := result["email"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected encrypted email result to be a map, got %T", result["email"])
	}
	if _, hasCipher := emailResult["ciphertext"]; !hasCipher {
		t.Error("missing ciphertext in encrypted result")
	}
}

func TestProcessPayloadValidationError(t *testing.T) {
	eng := setupTestEngine()

	sourceID := "src-1"

	ruleSet := &db.RuleSet{
		TargetFields: map[string]db.MappingTargetField{
			"tf-1": {ID: "tf-1", FieldName: "email", Encrypted: false},
		},
		Rules: []db.MappingRule{
			{
				SourceID:        "src-1",
				TargetFieldID:   "tf-1",
				SourceField:     "Mail",
				ValidationChain: json.RawMessage(`[{"name": "email", "parameters": {}}]`),
				ParsedValidations: []db.RuleStep{{Name: "email"}},
			},
		},
	}

	payload := map[string]interface{}{
		"Mail": "invalid-email",
	}

	masterKey := make([]byte, 32)
	wrappedKey := make([]byte, 32)

	result, errs := eng.ProcessPayload(payload, sourceID, ruleSet, masterKey, wrappedKey)
	if len(errs) != 1 {
		t.Fatalf("expected 1 pipeline error, got %d", len(errs))
	}

	if errs[0].FailedField != "Mail" || errs[0].RuleName != "validation" {
		t.Errorf("unexpected error details: %v", errs[0])
	}

	if _, exists := result["email"]; exists {
		t.Error("result should not contain 'email' due to validation failure")
	}
}
