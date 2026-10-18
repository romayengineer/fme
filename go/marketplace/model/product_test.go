package model

import "testing"

func TestProductGone(t *testing.T) {
	pdp := func(container any) map[string]any {
		return map[string]any{"data": map[string]any{"viewer": map[string]any{"marketplace_product_details_page": container}}}
	}

	tests := []struct {
		name       string
		data       any
		wantGone   bool
		wantReason string
	}{
		{
			name: "valid listing",
			data: pdp(map[string]any{"target": map[string]any{"id": "123"}}),
		},
		{
			name:       "null container",
			data:       pdp(nil),
			wantGone:   true,
			wantReason: "null_container",
		},
		{
			name:       "container without target",
			data:       pdp(map[string]any{"id": "page"}),
			wantGone:   true,
			wantReason: "no_target",
		},
		{
			name:       "target null",
			data:       pdp(map[string]any{"target": nil}),
			wantGone:   true,
			wantReason: "no_target",
		},
		{
			name:       "target without id",
			data:       pdp(map[string]any{"target": map[string]any{"title": "x"}}),
			wantGone:   true,
			wantReason: "no_target",
		},
		{
			name: "rate limit is not gone",
			data: map[string]any{"errors": []any{map[string]any{"message": "Rate limit exceeded"}}},
		},
		{
			name: "login wall is not gone",
			data: map[string]any{"data": map[string]any{"viewer": nil}},
		},
		{
			name: "unrelated payload is not gone",
			data: map[string]any{"unrelated": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, got := ProductGone(tt.data)
			if got != tt.wantGone {
				t.Fatalf("ProductGone = %v, want %v", got, tt.wantGone)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
