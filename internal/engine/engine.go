/**
 * SPDX-FileComment: Transformation Layer Pipeline Engine
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file engine.go
 * @brief Orchestrates the decryption, transformation, validation, and encryption pipeline.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"mitm_transformation/internal/crypto"
	"mitm_transformation/internal/db"
)

// PipelineError represents a validation or transformation error suitable for DLQ logging.
type PipelineError struct {
	FailedField  string
	RuleName     string
	ErrorMessage string
}

func (e *PipelineError) Error() string {
	return fmt.Sprintf("field %s failed on rule %s: %s", e.FailedField, e.RuleName, e.ErrorMessage)
}

// PipelineEngine orchestrates the transformation and validation.
type PipelineEngine struct {
	registry *EngineRegistry
}

// NewPipelineEngine creates a new pipeline engine.
func NewPipelineEngine(registry *EngineRegistry) *PipelineEngine {
	return &PipelineEngine{registry: registry}
}

// MergePayloads merges multiple JSON object maps into a single Golden Record.
// In case of key conflicts, the later payload's value overwrites the earlier.
func MergePayloads(payloads ...map[string]interface{}) map[string]interface{} {
	goldenRecord := make(map[string]interface{})
	for _, payload := range payloads {
		for key, value := range payload {
			goldenRecord[key] = value
		}
	}
	return goldenRecord
}

// ProcessPayload takes a decoded JSON payload and applies the rules for a specific source.
// Target encryption is performed for fields where targetField.Encrypted is true.
func (p *PipelineEngine) ProcessPayload(payload map[string]interface{}, sourceID string, ruleSet *db.RuleSet, masterKey []byte, wrappedKey []byte) (map[string]interface{}, []PipelineError) {
	result := make(map[string]interface{})
	var errors []PipelineError

	// Find all rules for this source
	for _, rule := range ruleSet.Rules {
		if rule.SourceID != sourceID {
			continue
		}

		targetField, ok := ruleSet.TargetFields[rule.TargetFieldID]
		if !ok {
			continue
		}

		var val interface{}
		var exists bool
		for k, v := range payload {
			if strings.EqualFold(k, rule.SourceField) {
				val = v
				exists = true
				break
			}
		}
		if !exists {
			val = nil
		}

		// Apply Transformations
		transformedVal, transformErr := p.applyTransformations(val, rule.TransformationChain)
		if transformErr != nil {
			errors = append(errors, PipelineError{
				FailedField:  rule.SourceField,
				RuleName:     "transformation",
				ErrorMessage: transformErr.Error(),
			})
			continue
		}

		// Apply Validations
		valid, validateErr := p.applyValidations(transformedVal, rule.ValidationChain)
		if !valid || validateErr != nil {
			errMsg := "validation failed"
			if validateErr != nil {
				errMsg = validateErr.Error()
			}
			errors = append(errors, PipelineError{
				FailedField:  rule.SourceField,
				RuleName:     "validation",
				ErrorMessage: errMsg,
			})
			continue
		}

		// Encryption if needed
		if targetField.Encrypted && transformedVal != nil && transformedVal != "" {
			strVal := fmt.Sprintf("%v", transformedVal)
			ciphertext, nonce, encErr := crypto.EnvelopeEncrypt(masterKey, wrappedKey, []byte(strVal))
			if encErr != nil {
				errors = append(errors, PipelineError{
					FailedField:  rule.SourceField,
					RuleName:     "encryption",
					ErrorMessage: encErr.Error(),
				})
				continue
			}
			transformedVal = map[string]interface{}{
				"ciphertext": ciphertext,
				"nonce":      nonce,
			}
		}

		result[targetField.FieldName] = transformedVal
	}

	return result, errors
}

func (p *PipelineEngine) applyTransformations(val interface{}, chainRaw json.RawMessage) (interface{}, error) {
	if len(chainRaw) == 0 || string(chainRaw) == "null" {
		return val, nil
	}

	var chain []struct {
		Name       string                 `json:"name"`
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal(chainRaw, &chain); err != nil {
		return val, fmt.Errorf("invalid transformation chain: %w", err)
	}

	currentVal := val
	for _, step := range chain {
		if step.Name == "" {
			continue
		}
		fn, err := p.registry.GetTransform(step.Name)
		if err != nil {
			return nil, err
		}
		currentVal, err = fn(currentVal, step.Parameters)
		if err != nil {
			return nil, fmt.Errorf("step %s failed: %w", step.Name, err)
		}
	}
	return currentVal, nil
}

func (p *PipelineEngine) applyValidations(val interface{}, chainRaw json.RawMessage) (bool, error) {
	if len(chainRaw) == 0 || string(chainRaw) == "null" {
		return true, nil
	}

	var chain []struct {
		Name       string                 `json:"name"`
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal(chainRaw, &chain); err != nil {
		return false, fmt.Errorf("invalid validation chain: %w", err)
	}

	for _, step := range chain {
		if step.Name == "" {
			continue
		}
		fn, err := p.registry.GetValidate(step.Name)
		if err != nil {
			return false, err
		}
		valid, err := fn(val, step.Parameters)
		if !valid || err != nil {
			if err != nil {
				return false, fmt.Errorf("step %s failed: %w", step.Name, err)
			}
			return false, fmt.Errorf("step %s returned false", step.Name)
		}
	}
	return true, nil
}
