package extract

import "testing"

type productGoneCase struct {
	name       string
	data       any
	wantGone   bool
	wantReason string
}

func TestProductGone(t *testing.T) {
	for _, tt := range productGoneCases() {
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

func pdpContainer(container any) map[string]any {
	return map[string]any{"data": map[string]any{"viewer": map[string]any{"marketplace_product_details_page": container}}}
}

func productGoneCases() []productGoneCase {
	return []productGoneCase{
		{name: "valid listing", data: pdpContainer(map[string]any{"target": map[string]any{"id": "123"}})},
		{name: "null container", data: pdpContainer(nil), wantGone: true, wantReason: "null_container"},
		{name: "container without target", data: pdpContainer(map[string]any{"id": "page"}), wantGone: true, wantReason: "no_target"},
		{name: "target null", data: pdpContainer(map[string]any{"target": nil}), wantGone: true, wantReason: "no_target"},
		{name: "target without id", data: pdpContainer(map[string]any{"target": map[string]any{"title": "x"}}), wantGone: true, wantReason: "no_target"},
		{name: "rate limit is not gone", data: map[string]any{"errors": []any{map[string]any{"message": "Rate limit exceeded"}}}},
		{name: "login wall is not gone", data: map[string]any{"data": map[string]any{"viewer": nil}}},
		{name: "unrelated payload is not gone", data: map[string]any{"unrelated": true}},
	}
}

func TestIsErrorRateLimitIgnoresNonStringMessages(t *testing.T) {
	tests := []struct {
		name string
		data any
		want bool
	}{
		{
			name: "case insensitive match",
			data: map[string]any{"errors": []any{map[string]any{"message": "RATE LIMIT EXCEEDED"}}},
			want: true,
		},
		{
			name: "numeric message",
			data: map[string]any{"errors": []any{map[string]any{"message": 123}}},
		},
		{
			name: "object message",
			data: map[string]any{"errors": []any{map[string]any{"message": map[string]any{"text": "rate limit exceeded"}}}},
		},
		{
			name: "missing message",
			data: map[string]any{"errors": []any{map[string]any{"code": 123}}},
		},
		{
			name: "errors is not a list",
			data: map[string]any{"errors": "rate limit exceeded"},
		},
		{
			name: "nil data",
			data: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsErrorRateLimit(tt.data); got != tt.want {
				t.Errorf("IsErrorRateLimit = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTaxonomyPathJoinedSkipsNonStringValues(t *testing.T) {
	detail := map[string]any{
		"marketplace_listing_renderable_target": map[string]any{
			"seo_virtual_category": map[string]any{
				"taxonomy_path": []any{
					map[string]any{"seo_info": map[string]any{"seo_url": 123}},
					map[string]any{"seo_info": map[string]any{"seo_url": "vehicles"}},
					map[string]any{"seo_info": map[string]any{"seo_url": ""}},
				},
			},
		},
	}

	if got := GetTaxonomyPathJoined(detail); got != "vehicles" {
		t.Errorf("GetTaxonomyPathJoined = %v, want vehicles", got)
	}
}

func TestProductDetailsGetRejectsInvalidIDs(t *testing.T) {
	for _, id := range []any{nil, 123, map[string]any{"value": "123"}, ""} {
		t.Run("id type", func(t *testing.T) {
			data := map[string]any{
				"data": map[string]any{
					"viewer": map[string]any{
						"marketplace_product_details_page": map[string]any{
							"target": map[string]any{"id": id},
						},
					},
				},
			}

			if _, err := ProductDetailsGet(data); err == nil {
				t.Errorf("ProductDetailsGet(%#v) returned no error", id)
			}
		})
	}
}

func TestProductFromDataGetRejectsInvalidIDs(t *testing.T) {
	for _, id := range []any{nil, 123, map[string]any{"value": "123"}, ""} {
		t.Run("id type", func(t *testing.T) {
			data := map[string]any{
				"data": map[string]any{
					"node": map[string]any{"entity_id": id},
				},
			}

			if _, err := ProductFromDataGet(data); err == nil {
				t.Errorf("ProductFromDataGet(%#v) returned no error", id)
			}
		})
	}
}

func TestProductsFromSearchGetSkipsInvalidIDs(t *testing.T) {
	data := map[string]any{
		"data": map[string]any{
			"marketplace_search": map[string]any{
				"feed_units": map[string]any{
					"edges": []any{
						map[string]any{"node": map[string]any{"listing": map[string]any{"id": 123}}},
						map[string]any{"node": map[string]any{"listing": map[string]any{"id": "valid-id"}}},
					},
				},
			},
		},
	}

	products, err := ProductsFromSearchGet(data)
	if err != nil {
		t.Fatalf("ProductsFromSearchGet returned error: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("got %d products, want 1", len(products))
	}
	if products[0].ID != "valid-id" {
		t.Errorf("ID = %v, want valid-id", products[0].ID)
	}
}

func TestExtractAllIgnoresInvalidIDsWithoutPanicking(t *testing.T) {
	data := map[string]any{
		"data": map[string]any{
			"viewer": map[string]any{
				"marketplace_product_details_page": map[string]any{
					"target": map[string]any{"id": 123},
				},
			},
		},
	}

	if got := NewProductExtractors().ExtractAll(data); got != nil {
		t.Errorf("ExtractAll = %v, want nil", got)
	}
}
