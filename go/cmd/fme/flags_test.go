package main

import "testing"

func TestOptionalBoolDistinguishesAbsentFromFalse(t *testing.T) {
	var value optionalBool
	if value.ptr() != nil {
		t.Fatal("absent flag returned an override")
	}

	if err := value.Set("false"); err != nil {
		t.Fatalf("Set(false): %v", err)
	}
	if ptr := value.ptr(); ptr == nil || *ptr {
		t.Fatalf("ptr() = %v, want a pointer to false", ptr)
	}

	if err := value.Set("not-a-bool"); err == nil {
		t.Fatal("Set accepted an invalid bool")
	}
}
