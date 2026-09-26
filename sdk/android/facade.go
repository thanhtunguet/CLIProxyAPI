package android

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/managementasset"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	log "github.com/sirupsen/logrus"
)

// Runtime manages the embedded CLIProxyAPI service lifecycle for Android.
type Runtime struct {
	mu             sync.Mutex
	cancel         context.CancelFunc
	doneCh         chan struct{}
	running        bool
	desired        bool
	workspace      string
	host           string
	port           int
	lastError      string
	transitionTime int64
	logCleanup     func()
	panelState     string
}

// NewRuntime creates an uninitialized Runtime instance.
func NewRuntime() *Runtime {
	return &Runtime{
		host:       "0.0.0.0",
		port:       8317,
		panelState: "offline",
	}
}

// DefaultRuntime is the global singleton runtime used by Android JNI.
var DefaultRuntime = NewRuntime()

// Start starts the CLIProxyAPI service after validating inputs and waiting for listener readiness.
func (r *Runtime) Start(ctx context.Context, cfg BootstrapConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		if r.workspace == cfg.WorkspaceDir && r.host == cfg.BindHost && r.port == cfg.Port {
			return nil
		}
		// Config or port changed; stop previous instance first
		r.stopLocked()
	}

	if err := EnsureWorkspace(cfg.WorkspaceDir); err != nil {
		r.lastError = err.Error()
		return err
	}

	if err := BootstrapConfigYAML(cfg.WorkspaceDir, cfg); err != nil {
		r.lastError = err.Error()
		return err
	}

	staticDir := filepath.Join(cfg.WorkspaceDir, staticDirName)
	_ = EnsureBundledOfflineHTML(staticDir)
	_ = os.Setenv("MANAGEMENT_STATIC_PATH", filepath.Join(staticDir, managementasset.ManagementFileName))

	configPath := filepath.Join(cfg.WorkspaceDir, configFileName)
	// Keep the on-disk management secret aligned with the value the Android app
	// hands to both CLIProxyAPI and the CPA Usage Keeper. LoadConfig hashes the
	// plaintext and rewrites the file, so without this step a secret configured
	// in the app after the first launch never reaches an existing config.yaml.
	// The Keeper would then authenticate with a key the server rejects, and its
	// polling would trip the server's failed-attempt IP ban.
	if secret := strings.TrimSpace(cfg.ManagementSecret); secret != "" {
		if errSecret := config.SaveConfigPreserveCommentsUpdateNestedScalar(configPath, []string{"remote-management", "secret-key"}, secret); errSecret != nil {
			log.WithError(errSecret).Warn("failed to persist management secret to config.yaml")
		}
	}
	loadedCfg, err := config.LoadConfig(configPath)
	if err != nil {
		r.lastError = fmt.Sprintf("load config: %v", err)
		return fmt.Errorf("load config: %w", err)
	}

	// Canonical app-private auth-dir
	loadedCfg.AuthDir = filepath.Join(cfg.WorkspaceDir, authDirName)

	effectiveHost := strings.TrimSpace(loadedCfg.Host)
	if effectiveHost == "" {
		effectiveHost = "0.0.0.0"
	}
	effectivePort := loadedCfg.Port
	if effectivePort == 0 {
		effectivePort = 8317
	}

	effectiveCfg := BootstrapConfig{
		WorkspaceDir:          cfg.WorkspaceDir,
		BindHost:              effectiveHost,
		Port:                  effectivePort,
		APIKeys:               loadedCfg.APIKeys,
		ManagementSecret:      loadedCfg.RemoteManagement.SecretKey,
		ManagementAllowRemote: loadedCfg.RemoteManagement.AllowRemote,
		PanelRepository:       loadedCfg.RemoteManagement.PanelGitHubRepository,
		AutoUpdatePanel:       !loadedCfg.RemoteManagement.DisableAutoUpdatePanel,
	}

	if err := ValidateConfig(effectiveCfg); err != nil {
		r.lastError = err.Error()
		return err
	}

	if err := CheckPortAvailable(effectiveHost, effectivePort); err != nil {
		r.lastError = err.Error()
		return err
	}

	// Bounded logging with redaction
	secrets := append([]string{}, loadedCfg.APIKeys...)
	if loadedCfg.RemoteManagement.SecretKey != "" {
		secrets = append(secrets, loadedCfg.RemoteManagement.SecretKey)
	}
	if cfg.ManagementSecret != "" {
		secrets = append(secrets, cfg.ManagementSecret)
	}
	r.logCleanup = SetupLogging(cfg.WorkspaceDir, secrets)

	runCtx, runCancel := context.WithCancel(ctx)
	readyCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	builder := cliproxy.NewBuilder().
		WithConfig(loadedCfg).
		WithConfigPath(configPath).
		WithListenerReady(func() {
			select {
			case readyCh <- struct{}{}:
			default:
			}
		})

	service, err := builder.Build()
	if err != nil {
		runCancel()
		if r.logCleanup != nil {
			r.logCleanup()
		}
		r.lastError = fmt.Sprintf("build service: %v", err)
		return fmt.Errorf("build service: %w", err)
	}

	go func() {
		defer close(doneCh)
		if runErr := service.Run(runCtx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			select {
			case errCh <- runErr:
			default:
			}
			r.mu.Lock()
			r.running = false
			r.lastError = runErr.Error()
			r.transitionTime = time.Now().UnixMilli()
			r.mu.Unlock()
			log.Errorf("CLIProxyAPI service exited unexpectedly: %v", runErr)
		}
	}()

	// Wait for listener readiness or fast failure
	select {
	case <-readyCh:
		// Listener open and ready to accept traffic
	case runErr := <-errCh:
		runCancel()
		<-doneCh
		if r.logCleanup != nil {
			r.logCleanup()
		}
		r.lastError = runErr.Error()
		return fmt.Errorf("service failed to start: %w", runErr)
	case <-time.After(5 * time.Second):
		runCancel()
		<-doneCh
		if r.logCleanup != nil {
			r.logCleanup()
		}
		r.lastError = "startup timed out waiting for HTTP listener"
		return errors.New(r.lastError)
	case <-ctx.Done():
		runCancel()
		<-doneCh
		if r.logCleanup != nil {
			r.logCleanup()
		}
		return ctx.Err()
	}

	// Start management auto updater with facade context
	managementasset.SetCurrentConfig(loadedCfg)
	managementasset.StartAutoUpdater(runCtx, configPath)

	r.cancel = runCancel
	r.doneCh = doneCh
	r.running = true
	r.desired = true
	r.workspace = cfg.WorkspaceDir
	r.host = effectiveHost
	r.port = effectivePort
	r.lastError = ""
	r.transitionTime = time.Now().UnixMilli()
	r.panelState = GetPanelState(staticDir)

	return nil
}

// Stop terminates the running service gracefully.
func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.desired = false
	r.stopLocked()
	return nil
}

func (r *Runtime) stopLocked() {
	if !r.running && r.cancel == nil {
		return
	}

	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}

	if r.doneCh != nil {
		select {
		case <-r.doneCh:
		case <-time.After(10 * time.Second):
			log.Warn("CLIProxyAPI stop timed out waiting for goroutine exit")
		}
		r.doneCh = nil
	}

	if r.logCleanup != nil {
		r.logCleanup()
		r.logCleanup = nil
	}

	r.running = false
	r.transitionTime = time.Now().UnixMilli()
}

// Reload reconfigures the service, applying changes to YAML and restarting if active.
func (r *Runtime) Reload(ctx context.Context, cfg BootstrapConfig) error {
	if err := UpdateConfigYAML(cfg.WorkspaceDir, cfg); err != nil {
		return err
	}

	r.mu.Lock()
	wasRunning := r.running
	r.mu.Unlock()

	if wasRunning {
		return r.Start(ctx, cfg)
	}
	return nil
}

// Status returns a sanitized, secret-free status representation.
func (r *Runtime) Status() StatusResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	panelState := r.panelState
	authCount := 0
	if r.workspace != "" {
		panelState = GetPanelState(filepath.Join(r.workspace, staticDirName))
		authCount = CountAuthFiles(r.workspace)
	}

	mgmtURL := fmt.Sprintf("http://%s:%d/management.html", r.host, r.port)
	if r.host == "0.0.0.0" {
		mgmtURL = fmt.Sprintf("http://127.0.0.1:%d/management.html", r.port)
	}

	return StatusResult{
		Desired:          r.desired,
		Running:          r.running,
		Host:             r.host,
		Port:             r.port,
		PanelState:       panelState,
		AuthFileCount:    authCount,
		TransitionTimeMs: r.transitionTime,
		LastError:        r.lastError,
		LoopbackOnly:     IsLoopback(r.host),
		ManagementURL:    mgmtURL,
	}
}

// EnsurePanel triggers an asynchronous or synchronous check for latest management panel.
func (r *Runtime) EnsurePanel(ctx context.Context, repo string) (string, error) {
	r.mu.Lock()
	workspace := r.workspace
	r.mu.Unlock()

	if workspace == "" {
		return "missing", errors.New("workspace not initialized")
	}

	staticDir := filepath.Join(workspace, staticDirName)
	state, err := EnsureManagementAsset(ctx, staticDir, "", repo)
	r.mu.Lock()
	r.panelState = state
	r.mu.Unlock()
	return state, err
}

// Package-level forwarders for global DefaultRuntime

func Start(ctx context.Context, cfg BootstrapConfig) error {
	return DefaultRuntime.Start(ctx, cfg)
}

func Stop() error {
	return DefaultRuntime.Stop()
}

func Reload(ctx context.Context, cfg BootstrapConfig) error {
	return DefaultRuntime.Reload(ctx, cfg)
}

func Status() StatusResult {
	return DefaultRuntime.Status()
}

func EnsurePanel(ctx context.Context, repo string) (string, error) {
	return DefaultRuntime.EnsurePanel(ctx, repo)
}
