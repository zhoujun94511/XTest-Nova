package main

import (
	"testing"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/command"
)

func TestCommandTimeoutMatchesTransferSize(t *testing.T) {
	if commandTimeout(command.Invocation{Kind: command.Pull}) != 3*time.Minute {
		t.Fatal("pull timeout is not the transfer budget")
	}
	if commandTimeout(command.Invocation{Kind: command.ScreenshotCmd}) != 3*time.Minute {
		t.Fatal("screenshot timeout is not the transfer budget")
	}
	if commandTimeout(command.Invocation{Kind: command.Monkey}) != 30*time.Second {
		t.Fatal("monkey timeout changed")
	}
	if commandTimeout(command.Invocation{Kind: command.Doctor}) != 30*time.Second {
		t.Fatal("doctor timeout changed")
	}
}
