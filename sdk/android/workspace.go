package android

import (
	"bytes"
	_ "embed"
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

//go:embed default_config.yaml
var DefaultConfigYAML []byte

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

	var docNode yaml.Node
	if len(DefaultConfigYAML) > 0 {
		if err := yaml.Unmarshal(DefaultConfigYAML, &docNode); err != nil {
			return fmt.Errorf("failed to parse default config template: %w", err)
		}
	}

	if docNode.Kind != yaml.DocumentNode || len(docNode.Content) == 0 || docNode.Content[0].Kind != yaml.MappingNode {
		// Create fresh document with root mapping
		rootMap := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		docNode = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{rootMap},
		}
	}
	rootMap := docNode.Content[0]

	// Always enforce the app-private auth-dir on Android
	setMappingScalar(rootMap, "auth-dir", authDir, "!!str")

	if cfg.Port > 0 && cfg.Port != 8317 {
		setMappingScalar(rootMap, "port", fmt.Sprintf("%d", cfg.Port), "!!int")
	} else if findMappingValue(rootMap, "port") == nil {
		setMappingScalar(rootMap, "port", "8317", "!!int")
	}

	if strings.TrimSpace(cfg.BindHost) != "" {
		setMappingScalar(rootMap, "host", strings.TrimSpace(cfg.BindHost), "!!str")
	} else if findMappingValue(rootMap, "host") == nil {
		setMappingScalar(rootMap, "host", "0.0.0.0", "!!str")
	}

	if len(cfg.APIKeys) > 0 {
		setMappingStringSeq(rootMap, "api-keys", cfg.APIKeys)
	}
	setMappingStringSeq(rootMap, "trusted-proxies", cfg.TrustedProxies)

	rmNode := getOrCreateMapping(rootMap, "remote-management")
	if cfg.ManagementAllowRemote {
		setMappingScalar(rmNode, "allow-remote", "true", "!!bool")
	}
	if !cfg.AutoUpdatePanel {
		setMappingScalar(rmNode, "disable-auto-update-panel", "true", "!!bool")
	}
	setMappingScalar(rmNode, "disable-control-panel", "false", "!!bool")

	if strings.TrimSpace(cfg.ManagementSecret) != "" {
		setMappingScalar(rmNode, "secret-key", strings.TrimSpace(cfg.ManagementSecret), "!!str")
	}
	if strings.TrimSpace(cfg.PanelRepository) != "" {
		setMappingScalar(rmNode, "panel-github-repository", strings.TrimSpace(cfg.PanelRepository), "!!str")
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&docNode); err != nil {
		return fmt.Errorf("failed to marshal initial config: %w", err)
	}
	_ = enc.Close()

	return atomicWriteFile(configPath, buf.Bytes())
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

	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	if docNode.Kind != yaml.DocumentNode || len(docNode.Content) == 0 || docNode.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid YAML structure in %s", configPath)
	}
	rootMap := docNode.Content[0]

	authDir := filepath.Join(workspaceDir, authDirName)
	setMappingScalar(rootMap, "auth-dir", authDir, "!!str")

	if cfg.Port > 0 {
		setMappingScalar(rootMap, "port", fmt.Sprintf("%d", cfg.Port), "!!int")
	}
	if strings.TrimSpace(cfg.BindHost) != "" {
		setMappingScalar(rootMap, "host", strings.TrimSpace(cfg.BindHost), "!!str")
	}

	rmNode := getOrCreateMapping(rootMap, "remote-management")
	setMappingScalar(rmNode, "allow-remote", fmt.Sprintf("%t", cfg.ManagementAllowRemote), "!!bool")
	setMappingScalar(rmNode, "disable-auto-update-panel", fmt.Sprintf("%t", !cfg.AutoUpdatePanel), "!!bool")
	if strings.TrimSpace(cfg.ManagementSecret) != "" {
		setMappingScalar(rmNode, "secret-key", strings.TrimSpace(cfg.ManagementSecret), "!!str")
	}
	if strings.TrimSpace(cfg.PanelRepository) != "" {
		setMappingScalar(rmNode, "panel-github-repository", strings.TrimSpace(cfg.PanelRepository), "!!str")
	}

	if len(cfg.APIKeys) > 0 {
		setMappingStringSeq(rootMap, "api-keys", cfg.APIKeys)
	}
	setMappingStringSeq(rootMap, "trusted-proxies", cfg.TrustedProxies)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&docNode); err != nil {
		return fmt.Errorf("failed to encode updated config: %w", err)
	}
	_ = enc.Close()

	return atomicWriteFile(configPath, buf.Bytes())
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

func findMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setMappingScalar(mapping *yaml.Node, key, val, tag string) {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Value = val
			mapping.Content[i+1].Tag = tag
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val},
	)
}

func setMappingStringSeq(mapping *yaml.Node, key string, items []string) {
	seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, item := range items {
		seqNode.Content = append(seqNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: item,
		})
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = seqNode
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		seqNode,
	)
}

func getOrCreateMapping(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			if mapping.Content[i+1].Kind == yaml.MappingNode {
				return mapping.Content[i+1]
			}
			break
		}
	}
	newMap := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		newMap,
	)
	return newMap
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
