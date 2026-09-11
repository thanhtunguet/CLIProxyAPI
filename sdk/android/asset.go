package android

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/managementasset"
)

const bundledOfflineHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CLIProxyAPI Management Center</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 24px; display: flex; justify-content: center; align-items: center; min-height: 100vh; box-sizing: border-box; }
    .container { background: #1e293b; border: 1px solid #334155; border-radius: 12px; max-width: 520px; width: 100%; padding: 32px; box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.5); }
    .header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
    h1 { margin: 0; font-size: 20px; font-weight: 600; color: #fff; }
    .badge { background: #f59e0b; color: #0f172a; font-size: 12px; font-weight: 700; padding: 4px 10px; border-radius: 9999px; text-transform: uppercase; }
    p { color: #94a3b8; font-size: 14px; line-height: 1.6; margin: 12px 0; }
    .card { background: #0f172a; border-radius: 8px; padding: 14px 18px; margin: 18px 0; font-family: monospace; font-size: 13px; color: #38bdf8; word-break: break-all; }
    .footer { margin-top: 24px; padding-top: 16px; border-top: 1px solid #334155; font-size: 12px; color: #64748b; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>CLIProxyAPI Management Center</h1>
      <span class="badge">Offline</span>
    </div>
    <p>The CLIProxyAPI service is active on this device. However, the complete Management Center web application has not finished downloading yet or this device has no internet connection.</p>
    <div class="card">Status: Service running &bull; Waiting for asset download</div>
    <p>Once network access is established, the verified Management Center release will download automatically in the background. Refresh this page after reconnecting.</p>
    <div class="footer">Android Embedded CLIProxyAPI Runtime</div>
  </div>
</body>
</html>`

// EnsureBundledOfflineHTML writes the bundled fallback HTML to static/management.html if absent.
func EnsureBundledOfflineHTML(staticDir string) error {
	target := filepath.Join(staticDir, managementasset.ManagementFileName)
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	if err := os.MkdirAll(staticDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(bundledOfflineHTML), 0o644)
}

// GetPanelState inspects the management.html asset on disk and returns its state.
func GetPanelState(staticDir string) string {
	target := filepath.Join(staticDir, managementasset.ManagementFileName)
	data, err := os.ReadFile(target)
	if err != nil {
		return "missing"
	}
	content := string(data)
	if strings.Contains(content, "Offline") && strings.Contains(content, "Waiting for asset download") {
		return "offline"
	}
	return "ready"
}

// EnsureManagementAsset runs verified download and atomic replacement using upstream logic.
func EnsureManagementAsset(ctx context.Context, staticDir string, proxyURL string, repo string) (string, error) {
	if err := os.MkdirAll(staticDir, 0o700); err != nil {
		return "error", err
	}

	ok := managementasset.EnsureLatestManagementHTML(ctx, staticDir, proxyURL, repo)
	state := GetPanelState(staticDir)
	if !ok && state == "missing" {
		_ = EnsureBundledOfflineHTML(staticDir)
		state = "offline"
	}
	return state, nil
}
