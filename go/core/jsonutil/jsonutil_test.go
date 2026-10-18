package jsonutil

import "testing"

func TestLookupDistinguishesAbsentFromNull(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"present_null": nil,
			"value":        "x",
		},
	}

	if v, ok := Lookup(data, "a.present_null"); !ok || v != nil {
		t.Errorf("present null: got (%v, %v), want (nil, true)", v, ok)
	}
	if v, ok := Lookup(data, "a.value"); !ok || v != "x" {
		t.Errorf("value: got (%v, %v), want (x, true)", v, ok)
	}
	if v, ok := Lookup(data, "a.missing"); ok || v != nil {
		t.Errorf("missing: got (%v, %v), want (nil, false)", v, ok)
	}
	if v, ok := Lookup(data, "a.value.deeper"); ok || v != nil {
		t.Errorf("through non-map: got (%v, %v), want (nil, false)", v, ok)
	}
	if v, ok := Lookup(nil, "a"); ok || v != nil {
		t.Errorf("nil data: got (%v, %v), want (nil, false)", v, ok)
	}
}

func TestGetKeyStillReturnsNil(t *testing.T) {
	data := map[string]any{"a": map[string]any{"present_null": nil}}
	if v := GetKey(data, "a.present_null"); v != nil {
		t.Errorf("GetKey(present null) = %v, want nil", v)
	}
	if v := GetKey(data, "a.missing"); v != nil {
		t.Errorf("GetKey(missing) = %v, want nil", v)
	}
}
