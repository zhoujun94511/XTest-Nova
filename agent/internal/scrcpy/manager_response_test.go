package scrcpy

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitClipboardAckIgnoresUnrelatedAndMismatchedEvents(t *testing.T) {
	session := newSession(nil, nil)
	session.publish(map[string]any{"type": "clipboard", "text": "external change"})
	session.publish(map[string]any{"type": "uhidOutput"})
	session.publishClipboardAck(40)
	session.publishClipboardAck(42)

	event, err := session.waitClipboardAck(context.Background(), time.Second, 42)
	if err != nil {
		t.Fatal(err)
	}
	if event["sequence"] != uint64(42) {
		t.Fatalf("ack = %#v", event)
	}
	event, err = session.waitClipboardAck(context.Background(), time.Second, 40)
	if err != nil {
		t.Fatal(err)
	}
	if event["sequence"] != uint64(40) {
		t.Fatalf("ack = %#v", event)
	}
}

func TestWaitDeviceEventTimeoutAndCancellationRemainDistinct(t *testing.T) {
	session := newSession(nil, nil)
	if _, err := session.waitClipboardAck(context.Background(), time.Millisecond, 1); !errors.Is(err, errDeviceResponseTimeout) {
		t.Fatalf("timeout error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := session.waitClipboardAck(ctx, time.Second, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestWaitDeviceEventReportsClosedControlSession(t *testing.T) {
	session := newSession(nil, nil)
	session.close()
	if _, err := session.waitClipboardAck(context.Background(), time.Second, 1); !errors.Is(err, errControlSessionClosed) {
		t.Fatalf("closed error = %v", err)
	}
}

func TestClipboardAckQueueAppliesBackpressureWithoutDropping(t *testing.T) {
	session := newSession(nil, nil)
	for sequence := uint64(1); sequence <= clipboardAckQueueSize; sequence++ {
		if !session.publishClipboardAck(sequence) {
			t.Fatal("ack publish unexpectedly failed")
		}
	}
	published := make(chan bool, 1)
	go func() { published <- session.publishClipboardAck(clipboardAckQueueSize + 1) }()
	select {
	case <-published:
		t.Fatal("ack publish did not block when queue was full")
	case <-time.After(20 * time.Millisecond):
	}
	event, err := session.waitClipboardAck(context.Background(), time.Second, clipboardAckQueueSize+1)
	if err != nil {
		t.Fatal(err)
	}
	if event["sequence"] != uint64(clipboardAckQueueSize+1) {
		t.Fatalf("ack = %#v", event)
	}
	select {
	case ok := <-published:
		if !ok {
			t.Fatal("ack publish failed before close")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked ack publisher was not released")
	}
}

func TestClosingSessionReleasesBlockedClipboardAckPublisher(t *testing.T) {
	session := newSession(nil, nil)
	for sequence := uint64(1); sequence <= clipboardAckQueueSize; sequence++ {
		session.publishClipboardAck(sequence)
	}
	published := make(chan bool, 1)
	go func() { published <- session.publishClipboardAck(clipboardAckQueueSize + 1) }()
	session.close()
	select {
	case ok := <-published:
		if ok {
			t.Fatal("ack publish succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked ack publisher leaked after close")
	}
}

func TestOnlyOneSharedControlChannelCanOwnResponses(t *testing.T) {
	manager := New()
	if !manager.acquireControl() {
		t.Fatal("first control owner was rejected")
	}
	if manager.acquireControl() {
		t.Fatal("second control owner was accepted")
	}
	manager.releaseControl()
	if !manager.acquireControl() {
		t.Fatal("control ownership was not released")
	}
}
