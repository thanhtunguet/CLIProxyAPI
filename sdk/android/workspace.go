package android

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configFileName = "config.yaml"
	authDirName    = "auth"
	staticDirName  = "static"
	logsDirName    = "logs"
	runtimeDirName = "runtime"
)

// EnsureWorkspace guarantees all required workspace subdirectories exist.
func EnsureWorkspace(workspaceDir string) error {
	workspaceDir = filepath.Clean(strings.TrimSpace(workspaceDir))
	if workspaceDir == "" {
		return fmt.Errorf("workspace directory cannot be empty")
	}

	dirs := []string{
		workspaceDir,
		filepath.Join(workspaceDir, authDirName),
		filepath.Join(workspaceDir, staticDirName),
		filepath.Join(workspaceDir, logsDirName),
		filepath.Join(workspaceDir, runtimeDirName),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}
	return nil
}

// BootstrapConfigYAML creates the initial canonical config.yaml only if it does not already exist.
func BootstrapConfigYAML(workspaceDir string, cfg BootstrapConfig) error {
	if err := EnsureWorkspace(workspaceDir); err != nil {
		return err
	}

	configPath := filepath.Join(workspaceDir, configFileName)
	if _, err := os.Stat(configPath); err == nil {
		// File already exists; do not overwrite rich configuration
		return nil
	}

	authDir := filepath.Join(workspaceDir, authDirName)

	initialData := map[string]any{
		"port":     cfg.Port,
		"host":     cfg.BindHost,
		"auth-dir": authDir,
		"remote-management": map[string]any{
			"disable-control-panel":     false,
			"disable-auto-update-panel": !cfg.AutoUpdatePanel,
			"allow-remote":              cfg.ManagementAllowRemote,
		},
	}

	if len(cfg.APIKeys) > 0 {
		initialData["api-keys"] = cfg.APIKeys
	} else {
		initialData["api-keys"] = []string{}
	}

	rmMap := initialData["remote-management"].(map[string]any)
	if cfg.ManagementSecret != "" {
		rmMap["secret"] = cfg.ManagementSecret
	}
	if cfg.PanelRepository != "" {
		rmMap["panel-github-repository"] = cfg.PanelRepository
	}

	out, err := yaml.Marshal(initialData)
	if err != nil {
		return fmt.Errorf("failed to marshal initial config: %w", err)
	}

	return atomicWriteFile(configPath, out)
}

// UpdateConfigYAML modifies Android-managed settings in existing config.yaml without wiping rich settings.
func UpdateConfigYAML(workspaceDir string, cfg BootstrapConfig) error {
	configPath := filepath.Join(workspaceDir, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return BootstrapConfigYAML(workspaceDir, cfg)
		}
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed to parse %s: %w", configPath, err)
	}
	if root == nil {
		root = make(map[string]any)
	}

	root["port"] = cfg.Port
	root["host"] = cfg.BindHost
	root["auth-dir"] = filepath.Join(workspaceDir, authDirName)

	var rm map[string]any
	if existingRm, ok := root["remote-management"].(map[string]any); ok && existingRm != nil {
		rm = existingRm
	} else {
		rm = make(map[string]any)
	}

	rm["allow-remote"] = cfg.ManagementAllowRemote
	rm["disable-auto-update-panel"] = !cfg.AutoUpdatePanel
	if cfg.PanelRepository != "" {
		rm["panel-github-repository"] = cfg.PanelRepository
	}
	if cfg.ManagementSecret != "" {
		rm["secret"] = cfg.ManagementSecret
	}
	root["remote-management"] = rm

	if len(cfg.APIKeys) > 0 {
		root["api-keys"] = cfg.APIKeys
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	return atomicWriteFile(configPath, out)
}

// CountAuthFiles counts the number of JSON credentials present in workspace/auth.
func CountAuthFiles(workspaceDir string) int {
	authDir := filepath.Join(workspaceDir, authDirName)
	entries, err := os.ReadDir(authDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			count++
		}
	}
	return count
}

func atomicWriteFile(targetPath string, content []byte) error {
	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, "cliproxy-tmp-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("rename to target: %w", err)
	}
	return nil
}
