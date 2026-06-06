/**
 * SPDX-FileComment: Transformation Layer Validate Library
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file library.go
 * @brief Catalog of standard validation functions.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package validate

import (
	"fmt"
	"regexp"

	"mitm_transformation/internal/engine"
)

// RegisterAll registers all core validations into the provided registry.
func RegisterAll(registry *engine.EngineRegistry) {
	registry.RegisterValidate("not_null", NotNull)
	registry.RegisterValidate("regex_match", RegexMatch)
	registry.RegisterValidate("range_check", RangeCheck)
	registry.RegisterValidate("email", Email)
	registry.RegisterValidate("in_list", InList)
}

func NotNull(val interface{}, params map[string]interface{}) (bool, error) {
	if val == nil {
		return false, fmt.Errorf("value is null")
	}
	if str, ok := val.(string); ok && str == "" {
		return false, fmt.Errorf("value is an empty string")
	}
	return true, nil
}

func RegexMatch(val interface{}, params map[string]interface{}) (bool, error) {
	str, ok := val.(string)
	if !ok {
		// If it's not a string, we might fail or allow. Let's fail for regex.
		return false, fmt.Errorf("value is not a string")
	}

	patternRaw, ok := params["pattern"]
	if !ok {
		return false, fmt.Errorf("missing 'pattern' parameter")
	}
	pattern, ok := patternRaw.(string)
	if !ok {
		return false, fmt.Errorf("'pattern' parameter must be a string")
	}

	matched, err := regexp.MatchString(pattern, str)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}
	if !matched {
		return false, fmt.Errorf("value does not match pattern")
	}

	return true, nil
}

func RangeCheck(val interface{}, params map[string]interface{}) (bool, error) {
	var num float64
	switch v := val.(type) {
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case float64:
		num = v
	case float32:
		num = float64(v)
	default:
		return false, fmt.Errorf("value is not numeric")
	}

	minRaw, hasMin := params["min"]
	maxRaw, hasMax := params["max"]

	if hasMin {
		var min float64
		switch v := minRaw.(type) {
		case int:
			min = float64(v)
		case float64:
			min = v
		}
		if num < min {
			return false, fmt.Errorf("value %v is less than minimum %v", num, min)
		}
	}

	if hasMax {
		var max float64
		switch v := maxRaw.(type) {
		case int:
			max = float64(v)
		case float64:
			max = v
		}
		if num > max {
			return false, fmt.Errorf("value %v is greater than maximum %v", num, max)
		}
	}

	return true, nil
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func Email(val interface{}, params map[string]interface{}) (bool, error) {
	str, ok := val.(string)
	if !ok {
		return false, fmt.Errorf("value is not a string")
	}

	if !emailRegex.MatchString(str) {
		return false, fmt.Errorf("value is not a valid email address")
	}

	return true, nil
}

func InList(val interface{}, params map[string]interface{}) (bool, error) {
	str, ok := val.(string)
	if !ok {
		return false, fmt.Errorf("value is not a string")
	}

	allowedRaw, ok := params["allowed"]
	if !ok {
		return false, fmt.Errorf("missing 'allowed' parameter")
	}

	allowedList, ok := allowedRaw.([]interface{})
	if !ok {
		return false, fmt.Errorf("'allowed' parameter must be a list")
	}

	for _, item := range allowedList {
		if fmt.Sprintf("%v", item) == str {
			return true, nil
		}
	}

	return false, fmt.Errorf("value %s is not in the allowed list", str)
}
