package marketplace

import "testing"

func TestToInt(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "int", value: 10, want: 10, ok: true},
		{name: "int64", value: int64(11), want: 11, ok: true},
		{name: "float64", value: 12.9, want: 12, ok: true},
		{name: "float32", value: float32(13.9), want: 13, ok: true},
		{name: "string", value: "14", ok: false},
		{name: "nil", value: nil, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toInt(tt.value)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScrollDownZeroIsNoOp(t *testing.T) {
	pages := &Pages{}

	if err := pages.ScrollDown(0); err != nil {
		t.Fatalf("ScrollDown(0) = %v, want nil", err)
	}
}
