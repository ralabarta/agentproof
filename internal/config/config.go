package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ralabarta/agentproof/internal/apperr"
	"github.com/ralabarta/agentproof/internal/safefile"
)

const DirName = ".agentproof"

type Config struct {
	SchemaVersion string   `json:"schema_version"`
	DefaultAgent  string   `json:"default_agent"`
	TestResults   []string `json:"test_results"`
	RequireTests  bool     `json:"require_tests"`
	FailOn        string   `json:"fail_on"`
	LocalOnly     bool     `json:"local_only"`
}

func Default() Config {
	return Config{
		SchemaVersion: "agentproof.config/v1",
		DefaultAgent:  "auto",
		TestResults:   []string{},
		RequireTests:  false,
		FailOn:        "critical",
		LocalOnly:     true,
	}
}

func Init(root string, force bool) error {
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%w: already initialized; use --force to replace the configuration", apperr.ErrUsage)
	}
	b, err := json.MarshalIndent(Default(), "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := safefile.Write(path, b, 0o600); err != nil {
		return err
	}
	ignore := "inputs/\nruns/\nlatest.json\nmanifest.json\nevidence.json\nattestation.json\nreport.md\nreport.html\n"
	return safefile.Write(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o600)
}

func Load(root string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(root, DirName, "config.json"))
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
