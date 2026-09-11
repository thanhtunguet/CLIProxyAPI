package android

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	redactMu       sync.RWMutex
	redactLiterals []string

	bearerRegex = regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9_\-\.]{6,})`)
	skKeyRegex  = regexp.MustCompile(`(?i)sk-[A-Za-z0-9_\-\.]{6,}`)
)

// SetRedactedSecrets updates the list of sensitive literals to mask in all logs.
func SetRedactedSecrets(secrets []string) {
	redactMu.Lock()
	defer redactMu.Unlock()
	redactLiterals = nil
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if len(s) >= 4 {
			redactLiterals = append(redactLiterals, s)
		}
	}
}

// RedactLog sanitizes sensitive tokens, API keys, and passwords from log messages.
func RedactLog(msg string) string {
	if msg == "" {
		return ""
	}

	msg = bearerRegex.ReplaceAllString(msg, "Bearer [REDACTED]")
	msg = skKeyRegex.ReplaceAllString(msg, "sk-[REDACTED]")

	redactMu.RLock()
	literals := redactLiterals
	redactMu.RUnlock()

	for _, lit := range literals {
		msg = strings.ReplaceAll(msg, lit, "[REDACTED]")
	}
	return msg
}

type androidLogrusHook struct {
	fileWriter io.WriteCloser
}

func (h *androidLogrusHook) Levels() []log.Level {
	return log.AllLevels
}

func (h *androidLogrusHook) Fire(entry *log.Entry) error {
	msg, err := entry.String()
	if err != nil {
		msg = fmt.Sprintf("[%s] %s\n", entry.Level, entry.Message)
	}
	redacted := RedactLog(msg)

	if h.fileWriter != nil {
		_, _ = h.fileWriter.Write([]byte(redacted))
	}

	prio := 3 // DEBUG
	switch entry.Level {
	case log.PanicLevel, log.FatalLevel, log.ErrorLevel:
		prio = 6 // ERROR
	case log.WarnLevel:
		prio = 5 // WARN
	case log.InfoLevel:
		prio = 4 // INFO
	case log.DebugLevel, log.TraceLevel:
		prio = 3 // DEBUG
	}

	writeToLogcat(prio, "CLIProxyAPI", redacted)
	return nil
}

// SetupLogging initializes bounded file logging in workspace/logs and logcat forwarding.
func SetupLogging(workspaceDir string, secrets []string) (cleanup func()) {
	SetRedactedSecrets(secrets)

	logFilePath := filepath.Join(workspaceDir, logsDirName, "cliproxy.log")
	rollingLogger := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    5, // 5 MB
		MaxBackups: 3,
		MaxAge:     7, // 7 days
		Compress:   true,
	}

	hook := &androidLogrusHook{fileWriter: rollingLogger}
	log.AddHook(hook)

	return func() {
		_ = rollingLogger.Close()
	}
}
