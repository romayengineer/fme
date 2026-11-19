package main

import "testing"

func TestOptionalBoolAbsentReturnsNoOverride(t *testing.T) {
	var value optionalBool

	if ptr := value.ptr(); ptr != nil {
		t.Fatalf("ptr() = %v, want nil for an absent flag", ptr)
	}
}

func TestOptionalBoolParsesExplicitFalse(t *testing.T) {
	var value optionalBool
	if err := value.Set("false"); err != nil {
		t.Fatalf("Set(false): %v", err)
	}

	if ptr := value.ptr(); ptr == nil || *ptr {
		t.Fatalf("ptr() = %v, want a pointer to false", ptr)
	}
}

func TestOptionalBoolRejectsInvalidValue(t *testing.T) {
	var value optionalBool

	if err := value.Set("not-a-bool"); err == nil {
		t.Fatal("Set accepted an invalid bool")
	}
}
