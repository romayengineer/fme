package orderedmap

import (
	"encoding/json"
	"fmt"

	"github.com/romayengineer/fme/go/core/logging"
)

type OrderedMap struct {
	data  map[string]string
	order []string
}

func (om *OrderedMap) Set(key string, value string) {
	if _, exists := om.data[key]; !exists {
		om.order = append(om.order, key)
	}
	om.data[key] = value
}

func (om *OrderedMap) SetJsonString(key string, KeyIn string, value any) error {
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

	dataMap[KeyIn] = value

	jsonString, err := json.Marshal(dataMap)
	if err != nil {
		return fmt.Errorf("marshal ordered-map value: %w", err)
	}

	om.Set(key, string(jsonString))

	return nil
}

func (om *OrderedMap) Get(key string) (string, bool) {
	data, exists := om.data[key]
	return data, exists
}

func (om *OrderedMap) GetDefault(key string, def string) string {
	val, exists := om.Get(key)
	if exists {
		return val
	}
	return def
}

func (om *OrderedMap) Keys() []string {
	return om.order
}

func (om OrderedMap) Clone() OrderedMap {
	clone := NewOrderedMap()
	for _, key := range om.order {
		clone.Set(key, om.data[key])
	}
	return clone
}

func (om *OrderedMap) Print() {
	for _, key := range om.Keys() {
		value, _ := om.Get(key)
		logging.LogDebug0("Print", "entry", "key", key, "value", value)
	}
}

func (om *OrderedMap) Compare(om2 OrderedMap) {
	var v1 string
	var v2 string
	var exists bool
	equal := 0
	for _, k := range om.Keys() {
		if v2, exists = om2.data[k]; !exists {
			fmt.Printf("key changed %s\n", k)
		}
		if v1, exists = om.data[k]; !exists {
			fmt.Printf("key changed %s\n", k)
		}
		if v1 != v2 {
			fmt.Printf("key changed %s\n", k)
		} else {
			equal += 1
		}
	}
	fmt.Printf("keys equal: %02d\n\n\n", equal)
}

func NewOrderedMap() OrderedMap {
	return OrderedMap{
		data:  map[string]string{},
		order: []string{},
	}
}
