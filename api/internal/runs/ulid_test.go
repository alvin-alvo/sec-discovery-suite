package runs

import (
	"testing"
	"time"
)

func TestULID(t *testing.T) {
	id1 := NewULID()
	if len(id1) != 26 {
		t.Fatalf("expected 26 chars, got %d (%s)", len(id1), id1)
	}

	time.Sleep(1 * time.Millisecond)
	id2 := NewULID()
	if len(id2) != 26 {
		t.Fatalf("expected 26 chars, got %d (%s)", len(id2), id2)
	}

	if id1 >= id2 {
		t.Fatalf("expected id1 < id2 for monotonic generation, got %s >= %s", id1, id2)
	}
}
