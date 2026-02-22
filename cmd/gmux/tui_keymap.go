package main

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// Re-export keymap types from the library package for use in this main package
type sessionizeKeyMap = navkeymap.SessionizeKeyMap
type manageKeyMap = navkeymap.ManageKeyMap
type historyKeyMap = navkeymap.HistoryKeyMap
type windowsKeyMap = navkeymap.WindowsKeyMap

var sessionizeKeys = navkeymap.NewSessionizeKeyMap()
var manageKeys = navkeymap.NewManageKeyMap()
var historyKeys = navkeymap.NewHistoryKeyMap()
var windowsKeys = navkeymap.NewWindowsKeyMap()
