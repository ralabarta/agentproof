package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidatesFailOn(t *testing.T) {
	tests := []struct {
		name    string
		failOn  string
		wantErr bool
	}{
		{name: "critical", failOn: "critical"},
		{name: "high", failOn: "high"},
		{name: "medium", failOn: "medium"},
		{name: "low", failOn: "low"},
		{name: "none", failOn: "none"},
		{name: "invalid", failOn: "critcal", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, DirName)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatalf("create config directory: %v", err)
			}
			contents := fmt.Sprintf(`{"fail_on":%q}`, tt.failOn)
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contents), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load(root)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() error = nil, want invalid fail_on error")
				}
				if !strings.Contains(err.Error(), "fail_on") || !strings.Contains(err.Error(), tt.failOn) {
					t.Fatalf("Load() error = %q, want fail_on and invalid value %q", err, tt.failOn)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.FailOn != tt.failOn {
				t.Fatalf("Load().FailOn = %q, want %q", cfg.FailOn, tt.failOn)
			}
		})
	}
}
