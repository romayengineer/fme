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

// ExtractJsonFromBody parses either a single JSON object or newline-delimited JSON
// objects from a response body. It returns an empty slice when the body is empty or
// does not start with an object.
func ExtractJsonFromBody(body []byte) ([]any, error) {
	jsonDatas := []any{}
	// make sure the first byte is { (open curly brakets)
	if len(body) == 0 || body[0] != '{' {
		return jsonDatas, nil
	}

	var jsonData any
	if err := json.Unmarshal(body, &jsonData); err == nil {
		jsonDatas = append(jsonDatas, jsonData)
		return jsonDatas, nil
	}

	var lineData any
	for line := range strings.SplitSeq(string(body), "\n") {
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		lineByte := []byte(line)
		if err := json.Unmarshal(lineByte, &lineData); err == nil {
			jsonDatas = append(jsonDatas, lineData)
		}
	}

	return jsonDatas, nil
}
