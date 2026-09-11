package android

import (
	"strings"
	"testing"
)

func TestRedactLog(t *testing.T) {
	SetRedactedSecrets([]string{"my-super-secret-key-12345", "mgmt-pass-987"})

	input := "Request with Bearer sk-ant-api03-1234567890abcdef and secret my-super-secret-key-12345 and mgmt-pass-987"
	redacted := RedactLog(input)

	if strings.Contains(redacted, "my-super-secret-key-12345") {
		t.Error("secret was not redacted")
	}
	if strings.Contains(redacted, "mgmt-pass-987") {
		t.Error("management pass was not redacted")
	}
	if strings.Contains(redacted, "sk-ant-api03-1234567890abcdef") {
		t.Error("bearer token was not redacted")
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Error("expected [REDACTED] placeholder in log message")
	}
}
