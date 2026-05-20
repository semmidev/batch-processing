package uuid

import (
	"testing"
)

func TestNew(t *testing.T) {
	// Generate a UUID v7
	id := New()

	// Check that it's not the Nil UUID
	if id == Nil {
		t.Error("expected non-nil UUID")
	}

	// Check that the version is indeed 7
	version := id.Version()
	if version != 7 {
		t.Errorf("expected UUID version 7, got %d", version)
	}
}

func TestNewString(t *testing.T) {
	idStr := NewString()

	// Parse it back to verify correctness
	parsed, err := Parse(idStr)
	if err != nil {
		t.Fatalf("failed to parse generated UUID string: %v", err)
	}

	// Check version
	if parsed.Version() != 7 {
		t.Errorf("expected UUID version 7, got %d", parsed.Version())
	}
}
