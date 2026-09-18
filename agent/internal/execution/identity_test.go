package execution

import (
	"errors"
	"testing"
)

func TestCoordinatorFencesStaleOwner(t *testing.T) {
	coordinator := NewCoordinator()
	first, err := coordinator.Acquire("exploration", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	owned, err := coordinator.Acquire("replay", "run-2")
	if !errors.Is(err, ErrOwned) {
		t.Fatalf("second acquire error = %v", err)
	}
	if owned != (Identity{}) {
		t.Fatalf("owned acquire identity = %+v, want zero value", owned)
	}
	if err = coordinator.Validate(first.SessionID, "wrong"); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("owner validation error = %v", err)
	}
	if !coordinator.Release(first) {
		t.Fatal("owner did not release")
	}
	second, err := coordinator.Acquire("replay", "run-2")
	if err != nil {
		t.Fatal(err)
	}
	if coordinator.Release(first) {
		t.Fatal("stale generation released current owner")
	}
	if !coordinator.Release(second) {
		t.Fatal("current owner did not release")
	}
}

func TestReceiptStoreIsIdempotent(t *testing.T) {
	store := NewReceiptStore()
	input := ActionReceipt{StepID: "s1", ObservationID: "o1", SourceFingerprint: "page", ActionFingerprint: "tap"}
	first, duplicate, err := store.Accept(input)
	if err != nil || duplicate {
		t.Fatalf("first accept = %+v duplicate=%v err=%v", first, duplicate, err)
	}
	second, duplicate, err := store.Accept(input)
	if err != nil || !duplicate || second.AcceptedAt != first.AcceptedAt {
		t.Fatalf("duplicate = %+v duplicate=%v err=%v", second, duplicate, err)
	}
}

func TestReceiptStoreRejectsConflictingStepIdentity(t *testing.T) {
	store := NewReceiptStore()
	original := ActionReceipt{StepID: "s1", ObservationID: "o1", SourceFingerprint: "page", ActionFingerprint: "tap"}
	if _, _, err := store.Accept(original); err != nil {
		t.Fatal(err)
	}
	tests := []ActionReceipt{
		{StepID: "s1", ObservationID: "o2", SourceFingerprint: "page", ActionFingerprint: "tap"},
		{StepID: "s1", ObservationID: "o1", SourceFingerprint: "other", ActionFingerprint: "tap"},
		{StepID: "s1", ObservationID: "o1", SourceFingerprint: "page", ActionFingerprint: "swipe"},
	}
	for _, input := range tests {
		receipt, duplicate, err := store.Accept(input)
		if !errors.Is(err, ErrReceiptIdentityConflict) {
			t.Fatalf("accept(%+v) error = %v", input, err)
		}
		if duplicate {
			t.Fatalf("accept(%+v) reported duplicate", input)
		}
		if receipt.ObservationID != original.ObservationID ||
			receipt.SourceFingerprint != original.SourceFingerprint ||
			receipt.ActionFingerprint != original.ActionFingerprint {
			t.Fatalf("stored receipt changed: %+v", receipt)
		}
	}
}
