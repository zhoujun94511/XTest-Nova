package execution

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrReceiptIdentityConflict = errors.New("action receipt identity conflicts with existing step")

type ReceiptStatus string

const (
	ReceiptAccepted ReceiptStatus = "accepted"
	ReceiptRejected ReceiptStatus = "rejected"
	ReceiptExecuted ReceiptStatus = "executed"
	ReceiptFailed   ReceiptStatus = "failed"
)

type ActionReceipt struct {
	StepID             string        `json:"stepId"`
	ObservationID      string        `json:"observationId"`
	SourceFingerprint  string        `json:"sourceFingerprint"`
	ActionFingerprint  string        `json:"actionFingerprint"`
	Status             ReceiptStatus `json:"status"`
	AcceptedAt         time.Time     `json:"acceptedAt"`
	FinishedAt         *time.Time    `json:"finishedAt,omitempty"`
	SettledObservation string        `json:"settledObservationId,omitempty"`
	SettledFingerprint string        `json:"settledFingerprint,omitempty"`
	ErrorCode          string        `json:"errorCode,omitempty"`
	Error              string        `json:"error,omitempty"`
}

type ReceiptStore struct {
	mu       sync.Mutex
	receipts map[string]ActionReceipt
}

func NewReceiptStore() *ReceiptStore { return &ReceiptStore{receipts: map[string]ActionReceipt{}} }

// Accept returns the saved receipt when a step is retried. Callers must not
// execute the action when duplicate is true.
func (s *ReceiptStore) Accept(value ActionReceipt) (receipt ActionReceipt, duplicate bool, err error) {
	if value.StepID == "" || value.ObservationID == "" || value.SourceFingerprint == "" || value.ActionFingerprint == "" {
		return ActionReceipt{}, false, errors.New("incomplete action receipt identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.receipts[value.StepID]; ok {
		if existing.ObservationID != value.ObservationID ||
			existing.SourceFingerprint != value.SourceFingerprint ||
			existing.ActionFingerprint != value.ActionFingerprint {
			return existing, false, fmt.Errorf(
				"%w: step %q has observation/source/action %q/%q/%q, got %q/%q/%q",
				ErrReceiptIdentityConflict, value.StepID,
				existing.ObservationID, existing.SourceFingerprint, existing.ActionFingerprint,
				value.ObservationID, value.SourceFingerprint, value.ActionFingerprint,
			)
		}
		return existing, true, nil
	}
	value.Status = ReceiptAccepted
	value.AcceptedAt = time.Now().UTC()
	s.receipts[value.StepID] = value
	return value, false, nil
}

func (s *ReceiptStore) Finish(stepID string, status ReceiptStatus, code, message, settledID, settledFingerprint string) (ActionReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.receipts[stepID]
	if !ok {
		return ActionReceipt{}, false
	}
	now := time.Now().UTC()
	value.Status, value.FinishedAt, value.ErrorCode, value.Error = status, &now, code, message
	value.SettledObservation, value.SettledFingerprint = settledID, settledFingerprint
	s.receipts[stepID] = value
	return value, true
}

func (s *ReceiptStore) All() []ActionReceipt {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ActionReceipt, 0, len(s.receipts))
	for _, value := range s.receipts {
		result = append(result, value)
	}
	return result
}
