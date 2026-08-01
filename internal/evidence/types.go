package evidence

import "time"

const RunSchemaVersion = "agentproof.dev/run/v1"

type Run struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Objective     string            `json:"objective"`
	Agent         string            `json:"agent"`
	Model         string            `json:"model,omitempty"`
	Command       []string          `json:"command,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at"`
	DurationMS    int64             `json:"duration_ms"`
	ExitCode      int               `json:"exit_code"`
	Repository    Repository        `json:"repository"`
	Sessions      []Session         `json:"sessions,omitempty"`
	Usage         Usage             `json:"usage"`
	Tests         TestResult        `json:"tests"`
	Findings      []Finding         `json:"findings"`
	Impact        Impact            `json:"impact"`
	Claims        []Claim           `json:"claims"`
	Status        string            `json:"status"`
	Integrity     string            `json:"integrity"`
	WarningCount  int               `json:"warning_count"`
	BundleID      string            `json:"bundle_id,omitempty"`
	Completeness  Completeness      `json:"completeness"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type Completeness struct {
	Observed int     `json:"observed"`
	Required int     `json:"required"`
	Percent  float64 `json:"percent"`
	Complete bool    `json:"complete"`
}

type Repository struct {
	Root              string      `json:"root"`
	StartHead         string      `json:"start_head"`
	EndHead           string      `json:"end_head"`
	DirtyBefore       bool        `json:"dirty_before"`
	DirtyAfter        bool        `json:"dirty_after"`
	AssociationStatus Association `json:"association_status"`
	Changes           []Change    `json:"changes"`
	CommittedChanges  []Change    `json:"committed_changes"`
	WorkingChanges    []Change    `json:"working_tree_changes"`
	UncapturedPaths   []string    `json:"uncaptured_paths,omitempty"`
	Commits           []Commit    `json:"commits"`
}

type Change struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Binary  bool   `json:"binary"`
}

type Commit struct {
	Hash    string `json:"hash"`
	Summary string `json:"summary"`
}

type Session struct {
	Adapter       string   `json:"adapter"`
	File          string   `json:"file"`
	Digest        string   `json:"digest,omitempty"`
	State         State    `json:"state"`
	Reason        string   `json:"reason,omitempty"`
	ParserVersion string   `json:"parser_version"`
	SourceBytes   int64    `json:"source_bytes"`
	Models        []string `json:"models,omitempty"`
	PromptCount   int      `json:"prompt_count"`
	Tools         []string `json:"tools,omitempty"`
	Usage         Usage    `json:"usage"`
}

type Usage struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CachedTokens int64   `json:"cached_tokens"`
	CostUSD      float64 `json:"estimated_cost_usd"`
}

type TestResult struct {
	Ingested     bool           `json:"ingested"`
	Passed       bool           `json:"passed"`
	PassedTests  int            `json:"passed_tests"`
	FailedTests  int            `json:"failed_tests"`
	SkippedTests int            `json:"skipped_tests"`
	DurationMS   int64          `json:"duration_ms"`
	Summary      string         `json:"summary"`
	Artifacts    []TestArtifact `json:"artifacts,omitempty"`
}

type TestArtifact struct {
	Path          string `json:"path"`
	Format        string `json:"format"`
	Producer      string `json:"producer"`
	FormatVersion string `json:"format_version"`
	Digest        string `json:"digest,omitempty"`
	State         State  `json:"state"`
	Reason        string `json:"reason,omitempty"`
	PassedTests   int    `json:"passed_tests"`
	FailedTests   int    `json:"failed_tests"`
	SkippedTests  int    `json:"skipped_tests"`
	DurationMS    int64  `json:"duration_ms"`
}

type Finding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Source      string `json:"source"`
	RuleVersion string `json:"rule_version"`
	Result      string `json:"result"`
	Description string `json:"description"`
}

type Impact struct {
	ChangedComponents  []string `json:"changed_components"`
	AffectedComponents []string `json:"affected_components"`
	Edges              []Edge   `json:"edges"`
	Radius             int      `json:"radius"`
	Analyzer           string   `json:"analyzer"`
	Complete           bool     `json:"complete"`
	Unsupported        []string `json:"unsupported,omitempty"`
	Unknown            []string `json:"unknown,omitempty"`
	FilesExamined      int      `json:"files_examined"`
	BytesParsed        int64    `json:"bytes_parsed"`
	LimitReached       string   `json:"limit_reached,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Claim struct {
	Type       string          `json:"type"`
	Statement  string          `json:"statement"`
	Confidence ClaimConfidence `json:"confidence"`
	Evidence   string          `json:"evidence"`
}

type Attestation struct {
	SchemaVersion string    `json:"schema_version"`
	Algorithm     string    `json:"algorithm"`
	EvidenceFile  string    `json:"evidence_file"`
	EvidenceHash  string    `json:"evidence_hash"`
	BundleID      string    `json:"bundle_id"`
	CreatedAt     time.Time `json:"created_at"`
}
