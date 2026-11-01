package app

import "testing"

func boolPtr(value bool) *bool {
	return &value
}

// assertOptionalBool compares an optional bool.
func assertOptionalBool(t *testing.T, got, want *bool) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Fatalf("headlessFromEnv() = %v, want nil", *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("headlessFromEnv() = nil, want %v", *want)
	}
	if *got != *want {
		t.Errorf("headlessFromEnv() = %v, want %v", *got, *want)
	}
}

func TestHeadlessFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  *bool
	}{
		{name: "unset", want: nil},
		{name: "true", value: "true", set: true, want: boolPtr(true)},
		{name: "one", value: "1", set: true, want: boolPtr(true)},
		{name: "false", value: "false", set: true, want: boolPtr(false)},
		{name: "zero", value: "0", set: true, want: boolPtr(false)},
		{name: "empty", value: "", set: true, want: nil},
		{name: "invalid", value: "nope", set: true, want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unsetEnv(t, "FME_HEADLESS")
			if test.set {
				t.Setenv("FME_HEADLESS", test.value)
			}

			assertOptionalBool(t, headlessFromEnv(), test.want)
		})
	}
}

func TestResolveHeadlessFirstOverrideWins(t *testing.T) {
	if !ResolveHeadless(boolPtr(true), boolPtr(false)) {
		t.Error("ResolveHeadless(true, false) = false, want true")
	}
	if ResolveHeadless(nil, boolPtr(false)) {
		t.Error("ResolveHeadless(nil, false) = true, want false")
	}
}
