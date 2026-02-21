package main

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// Re-export keymap types from the library package for use in this main package
type sessionizeKeyMap = navkeymap.SessionizeKeyMap

var sessionizeKeys = navkeymap.NewSessionizeKeyMap()
