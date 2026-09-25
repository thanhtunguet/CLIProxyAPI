// Package android provides the Android runtime facade for CLIProxyAPI.
package android

// BootstrapConfig specifies the parameters used to initialize and configure CLIProxyAPI on Android.
type BootstrapConfig struct {
	WorkspaceDir          string   `json:"workspaceDir"`
	BindHost              string   `json:"bindHost"`
	Port                  int      `json:"port"`
	APIKeys               []string `json:"apiKeys"`
	TrustedProxies        []string `json:"trustedProxies"`
	ManagementSecret      string   `json:"managementSecret"`
	ManagementAllowRemote bool     `json:"managementAllowRemote"`
	PanelRepository       string   `json:"panelRepository"`
	AutoUpdatePanel       bool     `json:"autoUpdatePanel"`
}

// StatusResult exposes safe, sanitized runtime status to Android.
// Secrets, credentials, and API keys are deliberately excluded.
type StatusResult struct {
	Desired          bool   `json:"desired"`
	Running          bool   `json:"running"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	PanelState       string `json:"panelState"` // "ready", "downloading", "offline", "error"
	AuthFileCount    int    `json:"authFileCount"`
	TransitionTimeMs int64  `json:"transitionTimeMs"`
	LastError        string `json:"lastError"`
	LoopbackOnly     bool   `json:"loopbackOnly"`
	ManagementURL    string `json:"managementURL"`
}
