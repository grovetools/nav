package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSessionOperations(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-session-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Run("create test session", func(t *testing.T) {
		cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "sleep", "10")
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create test session: %v", err)
		}
	})

	t.Run("SessionExists returns true for existing session", func(t *testing.T) {
		exists, err := client.SessionExists(ctx, sessionName)
		if err != nil {
			t.Errorf("SessionExists failed: %v", err)
		}
		if !exists {
			t.Error("Expected session to exist, but it doesn't")
		}
	})

	t.Run("CapturePane captures initial content", func(t *testing.T) {
		content, err := client.CapturePane(ctx, sessionName)
		if err != nil {
			t.Errorf("CapturePane failed: %v", err)
		}
		if content == "" {
			t.Error("Expected captured content, got empty string")
		}
	})

	t.Run("KillSession terminates the session", func(t *testing.T) {
		err := client.KillSession(ctx, sessionName)
		if err != nil {
			t.Errorf("KillSession failed: %v", err)
		}
	})

	t.Run("SessionExists returns false after kill", func(t *testing.T) {
		exists, err := client.SessionExists(ctx, sessionName)
		if err != nil {
			t.Errorf("SessionExists failed: %v", err)
		}
		if exists {
			t.Error("Expected session to not exist after kill, but it does")
		}
	})

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

func TestSessionExistsNonExistent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	exists, err := client.SessionExists(ctx, "non-existent-session-name")
	if err != nil {
		t.Errorf("SessionExists should not return error for non-existent session: %v", err)
	}
	if exists {
		t.Error("Expected non-existent session to return false")
	}
}

func TestCapturePane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-capture-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testMessage := "Hello from tmux test"
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "bash", "-c", fmt.Sprintf("echo '%s'; sleep 1", testMessage))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	content, err := client.CapturePane(ctx, sessionName)
	if err != nil {
		t.Errorf("CapturePane failed: %v", err)
	}

	if !strings.Contains(content, testMessage) {
		t.Errorf("Expected captured content to contain '%s', got: %s", testMessage, content)
	}

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}