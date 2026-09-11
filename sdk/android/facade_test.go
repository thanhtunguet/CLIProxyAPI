package android

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRuntimeInitialState(t *testing.T) {
	r := NewRuntime()
	st := r.Status()

	if st.Running {
		t.Error("expected running to be false")
	}
	if st.Desired {
		t.Error("expected desired to be false")
	}
	if st.Port != 8317 {
		t.Errorf("expected default port 8317, got %d", st.Port)
	}
}

func TestRuntimeInvalidConfig(t *testing.T) {
	r := NewRuntime()
	tempDir, err := os.MkdirTemp("", "cliproxy-facade-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := BootstrapConfig{
		WorkspaceDir: tempDir,
		BindHost:     "127.0.0.1",
		Port:         8080, // Reserved MyHome port
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = r.Start(ctx, cfg)
	if err == nil {
		t.Error("expected error for reserved port 8080, got nil")
	}
	if r.Status().Running {
		t.Error("expected runtime not to be running")
	}
}
