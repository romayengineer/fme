// Package jsonutil provides helpers for walking and decoding the untyped JSON
// payloads returned by Facebook's GraphQL endpoints.
package jsonutil

import (
	"encoding/json"
	"strings"
)

// GetKey walks a dotted path into nested map[string]any data and returns the value,
// or nil if any segment is missing or not a map.
func GetKey(data any, path string) any {
	value, _ := Lookup(data, path)
	return value
}

// Lookup walks a dotted path into nested map[string]any data and returns the value
// at that path plus whether the full key chain exists. Unlike GetKey it distinguishes
// a present-but-null value (present=true, value=nil) from a missing key
// (present=false), which is needed to tell "target is null" from "no target field".
func Lookup(data any, path string) (any, bool) {
	keys := strings.Split(path, ".")

	current := data
	for _, key := range keys {
		dataMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		value, ok := dataMap[key]
		if !ok {
			return nil, false
		}
		current = value
	}

	return current, true
}

// ExtractJSONFromBody parses either a single JSON object or newline-delimited JSON
// objects from a response body. It returns an empty slice when the body is empty or
// does not start with an object.
func ExtractJSONFromBody(body []byte) ([]any, error) {
	// A response body always starts with { when it contains JSON objects.
	if len(body) == 0 || body[0] != '{' {
		return []any{}, nil
	}

	if jsonData, ok := decodeObject(body); ok {
		return []any{jsonData}, nil
	}

	return decodeLines(body), nil
}

// decodeObject decodes body as a single JSON object.
func decodeObject(body []byte) (any, bool) {
	var jsonData any
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return nil, false
	}
	return jsonData, true
}

// decodeLines decodes each newline-delimited JSON object in body, skipping
// lines that are not objects or do not parse.
func decodeLines(body []byte) []any {
	jsonDatas := []any{}
	var lineData any
	for line := range strings.SplitSeq(string(body), "\n") {
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		if err := json.Unmarshal([]byte(line), &lineData); err == nil {
			jsonDatas = append(jsonDatas, lineData)
		}
	}
	return jsonDatas
}
