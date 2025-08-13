package tmux

type LaunchOptions struct {
	SessionName      string
	WorkingDirectory string
	WindowName       string
	Panes            []PaneOptions
}

type PaneOptions struct {
	Command          string
	WorkingDirectory string
	SendKeys         string
}