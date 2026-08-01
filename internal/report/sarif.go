package report

import (
	"encoding/json"
	"time"

	"github.com/ralabarta/agentproof/internal/evidence"
)

// SARIF generates a SARIF 2.1.0 JSON report for IDE and CI integration.
func SARIF(run evidence.Run) ([]byte, error) {
	runData := sarifRun{
		Tool: sarifTool{
			Driver: sarifDriver{
				Name:           "AgentProof",
				Version:        "0.1.0",
				InformationURI: "https://github.com/ralabarta/agentproof",
				Rules:          buildRules(run.Findings),
			},
		},
		Results:      buildResults(run.Findings),
		Invocations:  buildInvocations(run),
		Artifacts:    buildArtifacts(run.Repository.Changes),
		ColumnKind:   "utf16CodeUnits",
		OriginalURI:  run.Repository.StartHead,
	}

	report := sarifReport{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs:    []sarifRun{runData},
	}

	return json.MarshalIndent(report, "", "  ")
}

type sarifReport struct {
	Version string      `json:"version"`
	Schema  string      `json:"$schema"`
	Runs    []sarifRun  `json:"runs"`
}

type sarifRun struct {
	Tool         sarifTool         `json:"tool"`
	Results      []sarifResult     `json:"results"`
	Invocations  []sarifInvocation `json:"invocations,omitempty"`
	Artifacts    []sarifArtifact   `json:"artifacts,omitempty"`
	ColumnKind   string            `json:"columnKind"`
	OriginalURI  string            `json:"originalUriBaseIds,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string       `json:"name"`
	Version        string       `json:"version"`
	InformationURI string       `json:"informationUri"`
	Rules          []sarifRule  `json:"rules"`
}

type sarifRule struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	ShortDescription sarifText        `json:"shortDescription"`
	FullDescription  sarifText        `json:"fullDescription,omitempty"`
	DefaultConfig    sarifRuleConfig  `json:"defaultConfiguration"`
	Properties       *sarifProperties `json:"properties,omitempty"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifProperties struct {
	Tags []string `json:"tags,omitempty"`
}

type sarifResult struct {
	RuleID    string             `json:"ruleId"`
	Level     string             `json:"level"`
	Message   sarifMessage       `json:"message"`
	Locations []sarifLocation    `json:"locations,omitempty"`
	Properties *sarifResultProps `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifResultProps struct {
	RuleVersion string `json:"ruleVersion,omitempty"`
	Source      string `json:"source,omitempty"`
	Result      string `json:"result,omitempty"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool              `json:"executionSuccessful"`
	StartTimeUTC        string            `json:"startTimeUtc,omitempty"`
	EndTimeUTC          string            `json:"endTimeUtc,omitempty"`
	ExitCode            int               `json:"exitCode"`
	Properties          *sarifInvocProps  `json:"properties,omitempty"`
}

type sarifInvocProps struct {
	Agent      string `json:"agent,omitempty"`
	Model      string `json:"model,omitempty"`
	Objective  string `json:"objective,omitempty"`
	Association string `json:"association,omitempty"`
}

type sarifArtifact struct {
	Location sarifArtifactLocation `json:"location"`
	Length   int                   `json:"length,omitempty"`
	Roles    []string              `json:"roles,omitempty"`
}

func buildRules(findings []evidence.Finding) []sarifRule {
	seen := make(map[string]bool)
	var rules []sarifRule

	for _, finding := range findings {
		if seen[finding.ID] {
			continue
		}
		seen[finding.ID] = true

		rules = append(rules, sarifRule{
			ID:   finding.ID,
			Name: finding.ID,
			ShortDescription: sarifText{
				Text: finding.Title,
			},
			FullDescription: sarifText{
				Text: finding.Description,
			},
			DefaultConfig: sarifRuleConfig{
				Level: mapSeverity(finding.Severity),
			},
			Properties: &sarifProperties{
				Tags: []string{"security", "agent-proof"},
			},
		})
	}

	return rules
}

func buildResults(findings []evidence.Finding) []sarifResult {
	var results []sarifResult

	for _, finding := range findings {
		result := sarifResult{
			RuleID: finding.ID,
			Level:  mapSeverity(finding.Severity),
			Message: sarifMessage{
				Text: finding.Title,
			},
			Properties: &sarifResultProps{
				RuleVersion: finding.RuleVersion,
				Source:      finding.Source,
				Result:      finding.Result,
			},
		}

		if finding.Path != "" {
			location := sarifLocation{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{
						URI: finding.Path,
					},
				},
			}
			if finding.Line > 0 {
				location.PhysicalLocation.Region = &sarifRegion{
					StartLine: finding.Line,
				}
			}
			result.Locations = []sarifLocation{location}
		}

		results = append(results, result)
	}

	return results
}

func buildInvocations(run evidence.Run) []sarifInvocation {
	successful := run.Status == "passed"
	exitCode := 0
	if run.Status == "failed" {
		exitCode = 1
	} else if run.Status == "warning" {
		exitCode = 0
	}

	var startTime, endTime string
	if run.DurationMS > 0 {
		now := time.Now().UTC()
		endTime = now.Format(time.RFC3339)
		startTime = now.Add(-time.Duration(run.DurationMS) * time.Millisecond).Format(time.RFC3339)
	}

	return []sarifInvocation{{
		ExecutionSuccessful: successful,
		StartTimeUTC:        startTime,
		EndTimeUTC:          endTime,
		ExitCode:            exitCode,
		Properties: &sarifInvocProps{
			Agent:       run.Agent,
			Model:       run.Model,
			Objective:   run.Objective,
			Association: string(run.Repository.AssociationStatus),
		},
	}}
}

func buildArtifacts(changes []evidence.Change) []sarifArtifact {
	var artifacts []sarifArtifact

	for _, change := range changes {
		artifact := sarifArtifact{
			Location: sarifArtifactLocation{
				URI: change.Path,
			},
			Roles: []string{"analysisTarget"},
		}

		if change.Status == "added" {
			artifact.Roles = append(artifact.Roles, "added")
		} else if change.Status == "modified" {
			artifact.Roles = append(artifact.Roles, "modified")
		} else if change.Status == "deleted" {
			artifact.Roles = append(artifact.Roles, "deleted")
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts
}

func mapSeverity(severity string) string {
	switch severity {
	case "critical":
		return "error"
	case "high":
		return "error"
	case "medium":
		return "warning"
	case "low":
		return "note"
	default:
		return "none"
	}
}
