package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWaitForSessionCloseSuccess(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-wait-close-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "sleep", "0.5")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	start := time.Now()
	err = client.WaitForSessionClose(timeoutCtx, sessionName, 100*time.Millisecond)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("WaitForSessionClose failed: %v", err)
	}

	if duration < 500*time.Millisecond {
		t.Errorf("Expected wait to take at least 500ms, but took %v", duration)
	}

	if duration > 1*time.Second {
		t.Errorf("Expected wait to take less than 1s, but took %v", duration)
	}

	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		t.Errorf("Failed to check session existence: %v", err)
	}
	if exists {
		t.Error("Session should not exist after WaitForSessionClose returns")
	}
}

func TestWaitForSessionCloseContextCancel(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-wait-cancel-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "sleep", "10")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = client.WaitForSessionClose(cancelCtx, sessionName, 50*time.Millisecond)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if err != context.DeadlineExceeded {
		// The error might be wrapped or different due to the command being killed
		if !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "deadline exceeded") {
			t.Errorf("Expected context cancellation error, got: %v", err)
		}
	}

	if duration < 200*time.Millisecond || duration > 300*time.Millisecond {
		t.Errorf("Expected wait to take about 200ms, but took %v", duration)
	}

	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		t.Errorf("Failed to check session existence: %v", err)
	}
	if !exists {
		t.Error("Session should still exist after context cancellation")
	}

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

func TestWaitForSessionCloseNonExistent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "non-existent-session-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = client.WaitForSessionClose(timeoutCtx, sessionName, 50*time.Millisecond)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("WaitForSessionClose failed: %v", err)
	}

	if duration > 100*time.Millisecond {
		t.Errorf("Expected immediate return for non-existent session, but took %v", duration)
	}
}