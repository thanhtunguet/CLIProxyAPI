package android

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWorkspaceEnsureAndBootstrap(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cliproxy-ws-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := BootstrapConfig{
		WorkspaceDir:          tempDir,
		BindHost:              "127.0.0.1",
		Port:                  8317,
		APIKeys:               []string{"key-1", "key-2"},
		ManagementSecret:      "my-secret",
		ManagementAllowRemote: false,
		AutoUpdatePanel:       true,
	}

	if err := BootstrapConfigYAML(tempDir, cfg); err != nil {
		t.Fatalf("BootstrapConfigYAML: %v", err)
	}

	configPath := filepath.Join(tempDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	if parsed["port"] != 8317 {
		t.Errorf("expected port 8317, got %v", parsed["port"])
	}
	if parsed["host"] != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %v", parsed["host"])
	}
	if parsed["auth-dir"] != filepath.Join(tempDir, "auth") {
		t.Errorf("expected auth-dir %s, got %v", filepath.Join(tempDir, "auth"), parsed["auth-dir"])
	}

	// Subdirectories exist
	for _, sub := range []string{"auth", "static", "logs", "runtime"} {
		p := filepath.Join(tempDir, sub)
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s to exist", p)
		}
	}

	// Idempotency: calling Bootstrap again must NOT overwrite modified fields
	parsed["custom-setting"] = "do-not-wipe-me"
	updatedYAML, _ := yaml.Marshal(parsed)
	_ = os.WriteFile(configPath, updatedYAML, 0o600)

	if err := BootstrapConfigYAML(tempDir, cfg); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	data2, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data2), "do-not-wipe-me") {
		t.Error("expected second bootstrap to preserve existing configuration")
	}

	// UpdateConfigYAML modifies Android settings without wiping custom settings
	cfg.Port = 9000
	cfg.ManagementAllowRemote = true
	if err := UpdateConfigYAML(tempDir, cfg); err != nil {
		t.Fatalf("UpdateConfigYAML: %v", err)
	}
	data3, _ := os.ReadFile(configPath)
	var parsed3 map[string]any
	_ = yaml.Unmarshal(data3, &parsed3)
	if parsed3["port"] != 9000 {
		t.Errorf("expected updated port 9000, got %v", parsed3["port"])
	}
	if parsed3["custom-setting"] != "do-not-wipe-me" {
		t.Error("expected custom setting to be retained after update")
	}
}

func TestCountAuthFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cliproxy-auth-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	authDir := filepath.Join(tempDir, "auth")
	_ = os.MkdirAll(authDir, 0o700)

	_ = os.WriteFile(filepath.Join(authDir, "openai.json"), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(authDir, "claude.json"), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(authDir, "ignore.txt"), []byte(""), 0o600)

	count := CountAuthFiles(tempDir)
	if count != 2 {
		t.Errorf("expected 2 auth files, got %d", count)
	}
}
