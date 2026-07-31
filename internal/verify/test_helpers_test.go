package verify

import "github.com/ralabarta/agentproof/internal/evidence"

func runWithTests(passed bool) evidence.Run {
	return evidence.Run{
		Tests:        evidence.TestResult{Ingested: true, Passed: passed},
		Completeness: evidence.Completeness{Observed: 1, Required: 1, Percent: 100, Complete: true},
		Impact:       evidence.Impact{Complete: true},
	}
}
