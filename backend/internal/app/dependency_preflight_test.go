package app

import (
	"context"
	"strings"
	"testing"

	"github.com/example/sistemaemgo/internal/config"
)

func TestCheckLocalDependenciesRejectsUnconfiguredBackends(t *testing.T) {
	cases := []struct {
		name    string
		config  config.Config
		message string
	}{
		{
			name: "no PostgreSQL",
			config: config.Config{RedisAddr: "127.0.0.1:6379"},
			message: "PostgreSQL connection is not configured",
		},
		{
			name: "Redis disabled",
			config: config.Config{DatabaseURL: "postgres://user:secret@127.0.0.1:5432/db", DisableRedis: true},
			message: "Redis security backend must be enabled",
		},
		{
			name: "no Redis",
			config: config.Config{DatabaseURL: "postgres://user:secret@127.0.0.1:5432/db"},
			message: "Redis security backend is not configured",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckLocalDependencies(context.Background(), tc.config)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q, got %v", tc.message, err)
			}
		})
	}
}
