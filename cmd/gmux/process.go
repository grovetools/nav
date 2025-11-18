package main

import (
	"os/exec"
	"strconv"
	"strings"

	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
)

// buildProcessCache builds a map of PID -> actual process name for all windows
// This is done once to avoid calling ps multiple times
func buildProcessCache(windows []tmuxclient.Window) map[int]string {
	cache := make(map[int]string)

	// Get all processes once
	cmd := exec.Command("ps", "-o", "pid,ppid,command")
	output, err := cmd.Output()
	if err != nil {
		return cache
	}

	// Build a map of parent PID -> child process
	parentToChild := make(map[string]string)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		ppid := fields[1]
		command := strings.Join(fields[2:], " ")

		// Extract a clean command name
		cleanCmd := extractCommandName(command)
		if cleanCmd != "" {
			parentToChild[ppid] = cleanCmd
		}
	}

	// For each window, find its child process
	for _, win := range windows {
		if win.PID == 0 {
			continue
		}

		pidStr := strconv.Itoa(win.PID)
		if childCmd, ok := parentToChild[pidStr]; ok {
			cache[win.PID] = childCmd
		}
	}

	return cache
}

// extractCommandName cleans up a command string to show just the relevant parts
func extractCommandName(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	// Get the first part (the executable)
	baseCmd := parts[0]

	// If it's a scripting language with a script path, include that
	if len(parts) > 1 && (baseCmd == "node" || baseCmd == "python" || baseCmd == "python3") {
		scriptPath := parts[1]
		// Extract just the script name from the path
		scriptParts := strings.Split(scriptPath, "/")
		scriptName := scriptParts[len(scriptParts)-1]
		return baseCmd + " " + scriptName
	}

	return baseCmd
}
