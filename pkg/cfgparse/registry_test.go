package cfgparse

import "testing"

func TestRegistry_UnknownFormat(t *testing.T) {
	_, errGet := GetParser("nonexistent")
	if errGet == nil {
		t.Fatal("GetParser(nonexistent) expected error, got nil")
	}
}
