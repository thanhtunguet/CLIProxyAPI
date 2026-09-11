package android

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	ReservedMyHomePort = 8080
	MinPort            = 1024
	MaxPort            = 65535
)

// IsLoopback checks if the provided host address represents a loopback interface.
func IsLoopback(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// CheckPortAvailable attempts to open a temporary TCP listener to verify the port is free.
func CheckPortAvailable(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d on %s is not available: %w", port, host, err)
	}
	_ = l.Close()
	return nil
}

// ValidateConfig enforces strict port, host, security, and collision rules before service start.
func ValidateConfig(cfg BootstrapConfig) error {
	if cfg.Port < MinPort || cfg.Port > MaxPort {
		return fmt.Errorf("port %d is out of valid range (%d-%d)", cfg.Port, MinPort, MaxPort)
	}

	if cfg.Port == ReservedMyHomePort {
		return fmt.Errorf("port %d is reserved for the MyHome internal HTTP server", ReservedMyHomePort)
	}

	host := strings.TrimSpace(cfg.BindHost)
	if host == "" {
		host = "127.0.0.1"
	}

	// Fail closed if exposed outside loopback without authentication
	if !IsLoopback(host) {
		hasKey := false
		for _, k := range cfg.APIKeys {
			if strings.TrimSpace(k) != "" {
				hasKey = true
				break
			}
		}
		if !hasKey {
			return errors.New("fail closed: non-loopback bind host requires at least one configured API key")
		}
		if strings.TrimSpace(cfg.ManagementSecret) == "" {
			return errors.New("fail closed: non-loopback bind host requires a management secret")
		}
	}

	return nil
}
