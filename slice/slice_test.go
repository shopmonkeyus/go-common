package slice

import (
	"testing"
)

func TestSliceContains(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !SliceContains(slice, "a") {
		t.Errorf("expected true, got false")
	}
	if SliceContains(slice, "d") {
		t.Errorf("expected false, got true")
	}
}

func TestSliceContainsAny(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !SliceContainsAny(slice, "a", "d") {
		t.Errorf("expected true, got false")
	}
	if SliceContainsAny(slice, "d", "e") {
		t.Errorf("expected false, got true")
	}
}

func TestSliceOmit(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := SliceOmit(slice, "a", "c")
	if len(result) != 1 || result[0] != "b" {
		t.Errorf(`expected ["b"], got %v`, result)
	}
}
