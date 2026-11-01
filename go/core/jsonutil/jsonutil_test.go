package jsonutil

import "testing"

func TestLookupDistinguishesAbsentFromNull(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"present_null": nil,
			"value":        "x",
		},
	}

	cases := []struct {
		name   string
		path   string
		want   any
		wantOK bool
	}{
		{name: "present null", path: "a.present_null", want: nil, wantOK: true},
		{name: "value", path: "a.value", want: "x", wantOK: true},
		{name: "missing key", path: "a.missing", want: nil, wantOK: false},
		{name: "through non-map", path: "a.value.deeper", want: nil, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLookup(t, data, tc.path, tc.want, tc.wantOK)
		})
	}

	t.Run("nil data", func(t *testing.T) {
		assertLookup(t, nil, "a", nil, false)
	})
}

func assertLookup(t *testing.T, data any, path string, want any, wantOK bool) {
	t.Helper()

	got, ok := Lookup(data, path)
	if ok != wantOK || got != want {
		t.Errorf("Lookup(%q) = (%v, %v), want (%v, %v)", path, got, ok, want, wantOK)
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
