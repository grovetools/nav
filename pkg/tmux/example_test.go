package tmux_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mattsolo1/grove-tmux/pkg/tmux"
)

func ExampleClient_Launch() {
	ctx := context.Background()

	// Create a new tmux client
	client, err := tmux.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	// Configure a new tmux session with multiple panes
	opts := tmux.LaunchOptions{
		SessionName:      "my-dev-session",
		WorkingDirectory: "/tmp",
		WindowName:       "development",
		Panes: []tmux.PaneOptions{
			{
				Command: "echo 'Starting development server...'",
			},
			{
				Command:          "echo 'Running tests...'",
				WorkingDirectory: "/var",
			},
			{
				SendKeys: "htop",
			},
		},
	}

	// Launch the session
	if err := client.Launch(ctx, opts); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Session launched successfully")
}

func ExampleClient_WaitForSessionClose() {
	ctx := context.Background()

	client, err := tmux.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	// Wait for a session to close, checking every 100ms
	err = client.WaitForSessionClose(ctx, "my-session", 100*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Session closed")
}