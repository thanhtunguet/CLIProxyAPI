//go:build !android || !cgo

package android

import "os"

func writeToLogcat(prio int, tag string, text string) {
	// In test or non-Android environments, optionally write to stderr if in debug
	if os.Getenv("DEBUG_TEST_LOGS") != "" {
		_, _ = os.Stderr.WriteString("[" + tag + "] " + text)
	}
}
