package execution

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Identity is the immutable ownership envelope shared by a single execution
// attempt and all evidence produced by it.
type Identity struct {
	RunID      string `json:"runId"`
	AttemptID  string `json:"attemptId"`
	SessionID  string `json:"sessionId"`
	OwnerToken string `json:"ownerToken,omitempty"`
	Generation uint64 `json:"generation"`
}

func NewIdentity(kind, requestID string, generation uint64) Identity {
	kind = strings.TrimSpace(kind)
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = fmt.Sprintf("%s-%d", kind, time.Now().UnixNano())
	}
	suffix := randomHex(16)
	runID := requestID
	return Identity{
		RunID: runID, AttemptID: runID + ":attempt-1",
		SessionID:  fmt.Sprintf("%s:%d:%s", kind, generation, suffix[:12]),
		OwnerToken: suffix, Generation: generation,
	}
}

func randomHex(bytes int) string {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return fmt.Sprintf("%032x", time.Now().UnixNano())
}

var ErrOwned = errors.New("device control is owned by another execution session")
var ErrOwnerMismatch = errors.New("execution session ownership mismatch")

// Coordinator serializes sessions that can mutate the device UI. It uses
// explicit ownership rather than a goroutine lock so async finalizers can
// safely release the exact generation they acquired.
type Coordinator struct {
	mu         sync.Mutex
	generation uint64
	active     *Identity
}

func NewCoordinator() *Coordinator { return &Coordinator{} }

func (c *Coordinator) Acquire(kind, requestID string) (Identity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		return Identity{}, ErrOwned
	}
	c.generation++
	identity := NewIdentity(kind, requestID, c.generation)
	c.active = &identity
	return identity, nil
}

func (c *Coordinator) Validate(sessionID, ownerToken string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil || c.active.SessionID != sessionID || c.active.OwnerToken != ownerToken {
		return ErrOwnerMismatch
	}
	return nil
}

func (c *Coordinator) Release(identity Identity) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil || c.active.SessionID != identity.SessionID || c.active.OwnerToken != identity.OwnerToken || c.active.Generation != identity.Generation {
		return false
	}
	c.active = nil
	return true
}

func (c *Coordinator) Active() (Identity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == nil {
		return Identity{}, false
	}
	return *c.active, true
}
