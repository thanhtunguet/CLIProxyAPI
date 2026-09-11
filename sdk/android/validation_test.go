package android

import "testing"

func TestValidationRules(t *testing.T) {
	tests := []struct {
		name      string
		cfg       BootstrapConfig
		wantError bool
	}{
		{
			name: "valid loopback default",
			cfg: BootstrapConfig{
				BindHost: "127.0.0.1",
				Port:     8317,
			},
			wantError: false,
		},
		{
			name: "reserved myhome port 8080",
			cfg: BootstrapConfig{
				BindHost: "127.0.0.1",
				Port:     8080,
			},
			wantError: true,
		},
		{
			name: "port below 1024",
			cfg: BootstrapConfig{
				BindHost: "127.0.0.1",
				Port:     80,
			},
			wantError: true,
		},
		{
			name: "port above 65535",
			cfg: BootstrapConfig{
				BindHost: "127.0.0.1",
				Port:     70000,
			},
			wantError: true,
		},
		{
			name: "non-loopback without api keys fails closed",
			cfg: BootstrapConfig{
				BindHost:         "0.0.0.0",
				Port:             8317,
				ManagementSecret: "secret123",
			},
			wantError: true,
		},
		{
			name: "non-loopback without management secret fails closed",
			cfg: BootstrapConfig{
				BindHost: "192.168.1.50",
				Port:     8317,
				APIKeys:  []string{"sk-valid-key"},
			},
			wantError: true,
		},
		{
			name: "non-loopback with both key and secret succeeds",
			cfg: BootstrapConfig{
				BindHost:         "192.168.1.50",
				Port:             8317,
				APIKeys:          []string{"sk-valid-key"},
				ManagementSecret: "mgmt-secret-pass",
			},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfig(tc.cfg)
			if tc.wantError && err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if !tc.wantError && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	if !IsLoopback("127.0.0.1") {
		t.Error("expected 127.0.0.1 to be loopback")
	}
	if !IsLoopback("localhost") {
		t.Error("expected localhost to be loopback")
	}
	if !IsLoopback("::1") {
		t.Error("expected ::1 to be loopback")
	}
	if IsLoopback("0.0.0.0") {
		t.Error("expected 0.0.0.0 not to be loopback")
	}
	if IsLoopback("192.168.1.100") {
		t.Error("expected 192.168.1.100 not to be loopback")
	}
}
