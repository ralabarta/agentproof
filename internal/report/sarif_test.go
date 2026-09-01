package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/evidence"
)

func TestSARIFGeneratesValidReport(t *testing.T) {
	run := evidence.Run{
		Status:     "passed",
		Objective:  "Test SARIF generation",
		Agent:      "claude",
		Model:      "opus-4",
		DurationMS: 5000,
		Repository: evidence.Repository{
			StartHead:         "abc123",
			AssociationStatus: "high",
			Changes: []evidence.Change{
				{Path: "internal/auth/token.go", Status: "modified", Added: 10, Deleted: 2},
			},
		},
		Findings: []evidence.Finding{
			{
				ID:          "AP-SECRET-001",
				Severity:    "critical",
				Title:       "AWS key detected",
				Path:        "config.go",
				Line:        42,
				Source:      "scan",
				RuleVersion: "1.0",
				Result:      "violation",
				Description: "Potential AWS access key found",
			},
		},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if parsed.Version != "2.1.0" {
		t.Errorf("Expected version 2.1.0, got %s", parsed.Version)
	}

	if len(parsed.Runs) != 1 {
		t.Fatalf("Expected 1 run, got %d", len(parsed.Runs))
	}

	sarifRun := parsed.Runs[0]
	if sarifRun.Tool.Driver.Name != "AgentProof" {
		t.Errorf("Expected tool name AgentProof, got %s", sarifRun.Tool.Driver.Name)
	}

	if len(sarifRun.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(sarifRun.Results))
	}

	result := sarifRun.Results[0]
	if result.RuleID != "AP-SECRET-001" {
		t.Errorf("Expected rule AP-SECRET-001, got %s", result.RuleID)
	}

	if result.Level != "error" {
		t.Errorf("Expected level error for critical severity, got %s", result.Level)
	}

	if len(result.Locations) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result.Locations))
	}

	location := result.Locations[0]
	if location.PhysicalLocation.ArtifactLocation.URI != "config.go" {
		t.Errorf("Expected URI config.go, got %s", location.PhysicalLocation.ArtifactLocation.URI)
	}

	if location.PhysicalLocation.Region == nil || location.PhysicalLocation.Region.StartLine != 42 {
		t.Errorf("Expected line 42, got %v", location.PhysicalLocation.Region)
	}
}

func TestSARIFMapsAllSeverityLevels(t *testing.T) {
	tests := []struct {
		severity string
		expected string
	}{
		{"critical", "error"},
		{"high", "error"},
		{"medium", "warning"},
		{"low", "note"},
		{"unknown", "none"},
	}

	for _, tt := range tests {
		result := mapSeverity(tt.severity)
		if result != tt.expected {
			t.Errorf("mapSeverity(%s) = %s, expected %s", tt.severity, result, tt.expected)
		}
	}
}

func TestSARIFDeduplicatesRules(t *testing.T) {
	run := evidence.Run{
		Findings: []evidence.Finding{
			{ID: "AP-001", Severity: "high", Title: "Finding 1", Description: "Desc 1"},
			{ID: "AP-001", Severity: "high", Title: "Finding 1", Description: "Desc 1"},
			{ID: "AP-002", Severity: "medium", Title: "Finding 2", Description: "Desc 2"},
		},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	rules := parsed.Runs[0].Tool.Driver.Rules
	if len(rules) != 2 {
		t.Errorf("Expected 2 unique rules, got %d", len(rules))
	}

	results := parsed.Runs[0].Results
	if len(results) != 3 {
		t.Errorf("Expected 3 results (no deduplication), got %d", len(results))
	}
}

func TestSARIFHandlesEmptyFindings(t *testing.T) {
	run := evidence.Run{
		Status:    "passed",
		Objective: "No findings",
		Findings:  []evidence.Finding{},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(parsed.Runs[0].Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(parsed.Runs[0].Results))
	}

	if len(parsed.Runs[0].Tool.Driver.Rules) != 0 {
		t.Errorf("Expected 0 rules, got %d", len(parsed.Runs[0].Tool.Driver.Rules))
	}
}

func TestSARIFMapsInvocationOutcomes(t *testing.T) {
	tests := []struct {
		name                string
		status              string
		executionSuccessful bool
		exitCode            int
	}{
		{name: "passed", status: "passed", executionSuccessful: true, exitCode: 0},
		{name: "warning", status: "warning", executionSuccessful: true, exitCode: 0},
		{name: "failed", status: "failed", executionSuccessful: false, exitCode: 1},
		{name: "unsupported status fails closed", status: "unsupported", executionSuccessful: false, exitCode: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invocations := buildInvocations(evidence.Run{Status: tt.status})
			if len(invocations) != 1 {
				t.Fatalf("buildInvocations() returned %d invocations, want 1", len(invocations))
			}

			invocation := invocations[0]
			if got := invocation.ExecutionSuccessful; got != tt.executionSuccessful {
				t.Errorf("executionSuccessful = %t, want %t", got, tt.executionSuccessful)
			}
			if got := invocation.ExitCode; got != tt.exitCode {
				t.Errorf("exitCode = %d, want %d", got, tt.exitCode)
			}
		})
	}
}

func TestSARIFIncludesInvocationMetadata(t *testing.T) {
	run := evidence.Run{
		Status:     "failed",
		Agent:      "codex",
		Model:      "gpt-5",
		Objective:  "Add feature",
		DurationMS: 10000,
		Repository: evidence.Repository{
			AssociationStatus: "medium",
		},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	invocations := parsed.Runs[0].Invocations
	if len(invocations) != 1 {
		t.Fatalf("Expected 1 invocation, got %d", len(invocations))
	}

	inv := invocations[0]
	if inv.ExecutionSuccessful {
		t.Error("Expected executionSuccessful=false for failed status")
	}

	if inv.ExitCode != 1 {
		t.Errorf("Expected exit code 1 for failed status, got %d", inv.ExitCode)
	}

	if inv.Properties.Agent != "codex" {
		t.Errorf("Expected agent codex, got %s", inv.Properties.Agent)
	}

	if inv.Properties.Model != "gpt-5" {
		t.Errorf("Expected model gpt-5, got %s", inv.Properties.Model)
	}
}

func TestSARIFUsesRecordedInvocationTimes(t *testing.T) {
	startedAt := time.Date(2026, time.September, 1, 12, 34, 56, 0, time.FixedZone("UTC-4", -4*60*60))
	finishedAt := startedAt.Add(10 * time.Second)
	run := evidence.Run{
		Status:     "passed",
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		DurationMS: 10000,
	}

	data, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	invocation := parsed.Runs[0].Invocations[0]
	if got, want := invocation.StartTimeUTC, "2026-09-01T16:34:56Z"; got != want {
		t.Errorf("startTimeUtc = %q, want recorded start %q", got, want)
	}
	if got, want := invocation.EndTimeUTC, "2026-09-01T16:35:06Z"; got != want {
		t.Errorf("endTimeUtc = %q, want recorded finish %q", got, want)
	}
}

func TestSARIFIncludesArtifactsWithRoles(t *testing.T) {
	run := evidence.Run{
		Repository: evidence.Repository{
			Changes: []evidence.Change{
				{Path: "new.go", Status: "added"},
				{Path: "old.go", Status: "modified"},
				{Path: "gone.go", Status: "deleted"},
			},
		},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	artifacts := parsed.Runs[0].Artifacts
	if len(artifacts) != 3 {
		t.Fatalf("Expected 3 artifacts, got %d", len(artifacts))
	}

	for _, art := range artifacts {
		hasTarget := false
		for _, role := range art.Roles {
			if role == "analysisTarget" {
				hasTarget = true
				break
			}
		}
		if !hasTarget {
			t.Errorf("Artifact %s missing analysisTarget role", art.Location.URI)
		}

		if strings.HasPrefix(art.Location.URI, "new.") {
			found := false
			for _, role := range art.Roles {
				if role == "added" {
					found = true
					break
				}
			}
			if !found {
				t.Error("Added artifact should have 'added' role")
			}
		}
	}
}

func TestSARIFHandlesFindingWithoutLocation(t *testing.T) {
	run := evidence.Run{
		Findings: []evidence.Finding{
			{ID: "AP-GLOBAL-001", Severity: "high", Title: "Global issue", Path: "", Line: 0},
		},
	}

	sarif, err := SARIF(run)
	if err != nil {
		t.Fatalf("SARIF generation failed: %v", err)
	}

	var parsed sarifReport
	if err := json.Unmarshal(sarif, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	result := parsed.Runs[0].Results[0]
	if len(result.Locations) != 0 {
		t.Errorf("Expected no locations for finding without path, got %d", len(result.Locations))
	}
}
