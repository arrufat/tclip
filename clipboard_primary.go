//go:build freebsd || linux || netbsd || openbsd || solaris || dragonfly

package main

import "github.com/arrufat/clipboard"

const hasPrimary = true

func setPrimary(enabled bool) {
	clipboard.Primary = enabled

}
