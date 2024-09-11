//go:build darwin || windows

package main

const hasPrimary = false

func setPrimary(enabled bool) {}
