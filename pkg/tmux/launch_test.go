package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLaunch(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-launch-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	opts := LaunchOptions{
		SessionName:      sessionName,
		WorkingDirectory: "/tmp",
		WindowName:       "test-window",
		Panes: []PaneOptions{
			{
				Command: "echo 'First pane'",
			},
			{
				Command:          "echo 'Second pane'",
				WorkingDirectory: "/var",
			},
			{
				SendKeys: "echo 'Third pane'",
			},
		},
	}

	err = client.Launch(ctx, opts)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	cmd := exec.Command("tmux", "list-panes", "-t", sessionName)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list panes: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 panes, got %d", len(lines))
	}

	// Just verify we have the expected number of panes
	// Don't check specific formatting as it varies between tmux versions

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

func TestLaunchSinglePane(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-single-pane-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	opts := LaunchOptions{
		SessionName: sessionName,
		Panes: []PaneOptions{
			{
				Command: "echo 'Only pane'",
			},
		},
	}

	err = client.Launch(ctx, opts)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	content, err := client.CapturePane(ctx, sessionName)
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	if !strings.Contains(content, "Only pane") {
		t.Errorf("Expected captured content to contain 'Only pane', got: %s", content)
	}

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

func TestLaunchEmptySession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available in PATH, skipping integration tests")
	}

	ctx := context.Background()
	sessionName := "test-empty-" + time.Now().Format("20060102150405")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	opts := LaunchOptions{
		SessionName: sessionName,
	}

	err = client.Launch(ctx, opts)
	if err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		t.Fatalf("Failed to check session existence: %v", err)
	}

	if !exists {
		t.Error("Expected session to exist after launch")
	}

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})
}

func TestLaunchWithoutSessionName(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	opts := LaunchOptions{}

	err = client.Launch(context.Background(), opts)
	if err == nil {
		t.Error("Expected error when launching without session name")
	}
}