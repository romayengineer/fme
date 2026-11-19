package orderedmap

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSetPreservesInsertionOrder(t *testing.T) {
	om := NewOrderedMap()
	om.Set("first", "1")
	om.Set("second", "2")
	om.Set("first", "updated")

	if got, want := om.Keys(), []string{"first", "second"}; !slices.Equal(got, want) {
		t.Errorf("Keys = %v, want %v", got, want)
	}
	if got, _ := om.Get("first"); got != "updated" {
		t.Errorf("first = %q, want updated", got)
	}
	if got := om.GetDefault("missing", "fallback"); got != "fallback" {
		t.Errorf("GetDefault = %q, want fallback", got)
	}
}

func TestSetJSONStringUpdatesNestedField(t *testing.T) {
	om := NewOrderedMap()
	om.Set("variables", `{"targetId":"old","other":1}`)

	if err := om.SetJSONString("variables", "targetId", "new"); err != nil {
		t.Fatalf("SetJSONString: %v", err)
	}

	raw, _ := om.Get("variables")
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["targetId"] != "new" {
		t.Errorf("targetId = %v, want new", got["targetId"])
	}
	if got["other"] != float64(1) {
		t.Errorf("other = %v, want 1", got["other"])
	}
}

func TestSetJSONStringRejectsInvalidInput(t *testing.T) {
	om := NewOrderedMap()
	om.Set("array", `[1,2]`)
	om.Set("bad", `{not-json}`)

	tests := []struct{ name, key string }{
		{name: "missing key", key: "missing"},
		{name: "non-object", key: "array"},
		{name: "invalid json", key: "bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := om.SetJSONString(test.key, "targetId", "x"); err == nil {
				t.Errorf("SetJSONString(%q) returned no error", test.key)
			}
		})
	}
}

func TestCloneIsIndependent(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", "1")
	om.Set("b", "2")

	clone := om.Clone()
	clone.Set("a", "changed")

	if got, _ := om.Get("a"); got != "1" {
		t.Errorf("original a = %q, want 1", got)
	}
	if got, _ := clone.Get("a"); got != "changed" {
		t.Errorf("clone a = %q, want changed", got)
	}
	if got, want := clone.Keys(), om.Keys(); !slices.Equal(got, want) {
		t.Errorf("clone keys = %v, want %v", got, want)
	}
}

func TestKeysReturnsCopy(t *testing.T) {
	om := NewOrderedMap()
	om.Set("a", "1")

	keys := om.Keys()
	keys[0] = "mutated"

	if got := om.Keys()[0]; got != "a" {
		t.Errorf("first key = %q, want a", got)
	}
}
