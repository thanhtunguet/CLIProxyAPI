package android

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/managementasset"
)

func TestBundledOfflineAsset(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cliproxy-asset-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	staticDir := filepath.Join(tempDir, "static")

	if state := GetPanelState(staticDir); state != "missing" {
		t.Errorf("expected missing, got %s", state)
	}

	if err := EnsureBundledOfflineHTML(staticDir); err != nil {
		t.Fatalf("EnsureBundledOfflineHTML: %v", err)
	}

	if state := GetPanelState(staticDir); state != "offline" {
		t.Errorf("expected offline, got %s", state)
	}

	target := filepath.Join(staticDir, managementasset.ManagementFileName)
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(content), "Offline") {
		t.Error("expected Offline text in bundled html")
	}

	// Overwrite with full panel HTML
	_ = os.WriteFile(target, []byte("<!DOCTYPE html><html><body><h1>Control Panel</h1></body></html>"), 0o644)
	if state := GetPanelState(staticDir); state != "ready" {
		t.Errorf("expected ready, got %s", state)
	}
}
