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
			contents := fmt.Sprintf(`{"schema_version":"agentproof.config/v1","fail_on":%q}`, tt.failOn)
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

func TestLoadValidatesSchemaVersion(t *testing.T) {
	tests := []struct {
		name        string
		contents    string
		wantVersion string
		wantErr     string
	}{
		{
			name:        "valid v1",
			contents:    `{"schema_version":"agentproof.config/v1","fail_on":"critical"}`,
			wantVersion: "agentproof.config/v1",
		},
		{
			name:     "missing",
			contents: `{"fail_on":"critical"}`,
			wantErr:  `invalid schema_version "": must be "agentproof.config/v1"`,
		},
		{
			name:     "unsupported",
			contents: `{"schema_version":"agentproof.config/v2","fail_on":"critical"}`,
			wantErr:  `invalid schema_version "agentproof.config/v2": must be "agentproof.config/v1"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, DirName)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatalf("create config directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tt.contents), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load(root)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("Load() error = %q, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.SchemaVersion != tt.wantVersion {
				t.Fatalf("Load().SchemaVersion = %q, want %q", cfg.SchemaVersion, tt.wantVersion)
			}
		})
	}
}
