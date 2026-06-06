/**
 * SPDX-FileComment: Transformation Layer Registry Pattern
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file registry.go
 * @brief Registry for mapping transformation and validation function names to Go functions.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package engine

import "fmt"

// TransformFunc defines the signature for a transformation function.
type TransformFunc func(val interface{}, params map[string]interface{}) (interface{}, error)

// ValidateFunc defines the signature for a validation function.
type ValidateFunc func(val interface{}, params map[string]interface{}) (bool, error)

// EngineRegistry holds the registered transformation and validation functions.
type EngineRegistry struct {
	transforms map[string]TransformFunc
	validators map[string]ValidateFunc
}

// NewEngineRegistry initializes a new registry.
func NewEngineRegistry() *EngineRegistry {
	return &EngineRegistry{
		transforms: make(map[string]TransformFunc),
		validators: make(map[string]ValidateFunc),
	}
}

// RegisterTransform adds a transformation function to the registry.
func (r *EngineRegistry) RegisterTransform(name string, fn TransformFunc) {
	r.transforms[name] = fn
}

// RegisterValidate adds a validation function to the registry.
func (r *EngineRegistry) RegisterValidate(name string, fn ValidateFunc) {
	r.validators[name] = fn
}

// GetTransform retrieves a transformation function by name.
func (r *EngineRegistry) GetTransform(name string) (TransformFunc, error) {
	fn, ok := r.transforms[name]
	if !ok {
		return nil, fmt.Errorf("transform function '%s' not found in registry", name)
	}
	return fn, nil
}

// GetValidate retrieves a validation function by name.
func (r *EngineRegistry) GetValidate(name string) (ValidateFunc, error) {
	fn, ok := r.validators[name]
	if !ok {
		return nil, fmt.Errorf("validate function '%s' not found in registry", name)
	}
	return fn, nil
}
