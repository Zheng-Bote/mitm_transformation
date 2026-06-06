/**
 * SPDX-FileComment: Transformation Layer Transform Library
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file library.go
 * @brief Catalog of standard transformation functions.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package transform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mitm_transformation/internal/engine"
)

// RegisterAll registers all core transformations into the provided registry.
func RegisterAll(registry *engine.EngineRegistry) {
	registry.RegisterTransform("trim_whitespace", TrimWhitespace)
	registry.RegisterTransform("to_upper", ToUpper)
	registry.RegisterTransform("to_lower", ToLower)
	registry.RegisterTransform("default_value", DefaultValue)
	registry.RegisterTransform("regex_replace", RegexReplace)
	registry.RegisterTransform("parse_date", ParseDate)
	registry.RegisterTransform("string_split", StringSplit)
	registry.RegisterTransform("cast_type", CastType)
}

func TrimWhitespace(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok {
		return val, nil
	}
	return strings.TrimSpace(str), nil
}

func ToUpper(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok {
		return val, nil
	}
	return strings.ToUpper(str), nil
}

func ToLower(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok {
		return val, nil
	}
	return strings.ToLower(str), nil
}

func DefaultValue(val interface{}, params map[string]interface{}) (interface{}, error) {
	if val != nil && val != "" {
		return val, nil
	}
	defVal, ok := params["value"]
	if !ok {
		return val, fmt.Errorf("missing 'value' parameter for default_value")
	}
	return defVal, nil
}

func RegexReplace(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok || val == "" {
		return val, nil
	}

	patternRaw, ok := params["pattern"]
	if !ok {
		return val, fmt.Errorf("missing 'pattern' parameter for regex_replace")
	}
	pattern, ok := patternRaw.(string)
	if !ok {
		return val, fmt.Errorf("'pattern' parameter must be a string")
	}

	replaceRaw, ok := params["replace"]
	if !ok {
		return val, fmt.Errorf("missing 'replace' parameter for regex_replace")
	}
	replace, ok := replaceRaw.(string)
	if !ok {
		return val, fmt.Errorf("'replace' parameter must be a string")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return val, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return re.ReplaceAllString(str, replace), nil
}

func ParseDate(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok || val == "" {
		return val, nil
	}

	inFormatRaw, ok := params["input_format"]
	if !ok {
		return val, fmt.Errorf("missing 'input_format' parameter")
	}
	inFormat, ok := inFormatRaw.(string)
	if !ok {
		return val, fmt.Errorf("'input_format' parameter must be a string")
	}

	outFormatRaw, ok := params["output_format"]
	if !ok {
		outFormatRaw = time.RFC3339
	}
	outFormat, ok := outFormatRaw.(string)
	if !ok {
		return val, fmt.Errorf("'output_format' parameter must be a string")
	}

	parsedTime, err := time.Parse(inFormat, str)
	if err != nil {
		return val, fmt.Errorf("failed to parse date: %w", err)
	}

	return parsedTime.Format(outFormat), nil
}

func StringSplit(val interface{}, params map[string]interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok || val == "" {
		return val, nil
	}

	sepRaw, ok := params["separator"]
	if !ok {
		return val, fmt.Errorf("missing 'separator' parameter")
	}
	separator, ok := sepRaw.(string)
	if !ok {
		return val, fmt.Errorf("'separator' parameter must be a string")
	}

	idxRaw, ok := params["index"]
	if !ok {
		return val, fmt.Errorf("missing 'index' parameter")
	}

	var index int
	switch v := idxRaw.(type) {
	case int:
		index = v
	case float64:
		index = int(v)
	default:
		return val, fmt.Errorf("'index' parameter must be numeric")
	}

	parts := strings.Split(str, separator)
	if index < 0 || index >= len(parts) {
		return nil, fmt.Errorf("index %d out of bounds for split result of length %d", index, len(parts))
	}

	return parts[index], nil
}

func CastType(val interface{}, params map[string]interface{}) (interface{}, error) {
	if val == nil {
		return nil, nil
	}

	targetRaw, ok := params["target_type"]
	if !ok {
		return val, fmt.Errorf("missing 'target_type' parameter")
	}
	targetType, ok := targetRaw.(string)
	if !ok {
		return val, fmt.Errorf("'target_type' parameter must be a string")
	}

	strVal := fmt.Sprintf("%v", val)

	switch strings.ToLower(targetType) {
	case "int", "integer":
		i, err := strconv.Atoi(strVal)
		if err != nil {
			return val, fmt.Errorf("failed to cast to integer: %w", err)
		}
		return i, nil
	case "float", "float64":
		f, err := strconv.ParseFloat(strVal, 64)
		if err != nil {
			return val, fmt.Errorf("failed to cast to float: %w", err)
		}
		return f, nil
	case "bool", "boolean":
		b, err := strconv.ParseBool(strVal)
		if err != nil {
			return val, fmt.Errorf("failed to cast to bool: %w", err)
		}
		return b, nil
	case "string":
		return strVal, nil
	default:
		return val, fmt.Errorf("unsupported target_type: %s", targetType)
	}
}
