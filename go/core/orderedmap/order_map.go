// Package orderedmap provides a string map that preserves insertion order.
//
// Facebook GraphQL payloads are order-sensitive when replayed, so the request
// body must keep the original key order.
package orderedmap

import (
	"encoding/json"
	"fmt"
)

// OrderedMap is a string map that iterates in insertion order.
type OrderedMap struct {
	data  map[string]string
	order []string
}

// Set stores value under key, appending the key the first time it is seen.
func (om *OrderedMap) Set(key string, value string) {
	if om.data == nil {
		om.data = map[string]string{}
	}
	if _, exists := om.data[key]; !exists {
		om.order = append(om.order, key)
	}
	om.data[key] = value
}

// SetJSONString decodes the JSON object stored under key, sets keyIn to value,
// and stores the re-encoded object back under key.
func (om *OrderedMap) SetJSONString(key string, keyIn string, value any) error {
	val, exists := om.Get(key)
	if !exists {
		return fmt.Errorf("key %q does not exist in ordered map", key)
	}

	var jsonData any
	if err := json.Unmarshal([]byte(val), &jsonData); err != nil {
		return fmt.Errorf("key %q is not a JSON string", key)
	}

	dataMap, ok := jsonData.(map[string]any)
	if !ok {
		return fmt.Errorf("key %q is not a JSON object", key)
	}

	dataMap[keyIn] = value

	jsonString, err := json.Marshal(dataMap)
	if err != nil {
		return fmt.Errorf("marshal ordered-map value: %w", err)
	}

	om.Set(key, string(jsonString))

	return nil
}

// Get returns the value stored under key and whether it exists.
func (om *OrderedMap) Get(key string) (string, bool) {
	data, exists := om.data[key]
	return data, exists
}

// GetDefault returns the value stored under key, or def when it is missing.
func (om *OrderedMap) GetDefault(key string, def string) string {
	val, exists := om.Get(key)
	if exists {
		return val
	}
	return def
}

// Keys returns the keys in insertion order. The caller owns the returned slice.
func (om *OrderedMap) Keys() []string {
	return append([]string(nil), om.order...)
}

// Clone returns an independent copy that preserves key order.
func (om *OrderedMap) Clone() OrderedMap {
	clone := NewOrderedMap()
	for _, key := range om.order {
		clone.Set(key, om.data[key])
	}
	return clone
}

// NewOrderedMap returns an empty OrderedMap ready for use.
func NewOrderedMap() OrderedMap {
	return OrderedMap{
		data:  map[string]string{},
		order: []string{},
	}
}
