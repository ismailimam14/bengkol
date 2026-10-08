package logger_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bengkol/backend/pkg/logger"
)

func TestLogger_StructuredOutput(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	log.Info("test message", "key1", "val1")

	output := buf.String()
	if !strings.Contains(output, "test message") || !strings.Contains(output, "val1") {
		t.Errorf("expected output to contain message and attributes, got: %s", output)
	}
}

func TestLogger_WithContext(t *testing.T) {
	envs := []string{"production", "prod", "staging", "uat"}

	for _, env := range envs {
		t.Run(env, func(t *testing.T) {
			var buf bytes.Buffer
			log := logger.NewWithOutput(env, "info", &buf)

			ctx := context.WithValue(context.Background(), logger.RequestIDKey, "req-xyz-999")
			ctxLog := log.WithContext(ctx)

			ctxLog.Info("request processed")

			output := buf.String()
			if !strings.Contains(output, "req-xyz-999") {
				t.Errorf("expected JSON log output for %s to contain request_id attribute, got: %s", env, output)
			}
			// Confirm JSON format has braces
			if !strings.HasPrefix(strings.TrimSpace(output), "{") {
				t.Errorf("expected JSON log output to start with '{' for %s, got: %s", env, output)
			}
		})
	}
}
