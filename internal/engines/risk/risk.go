// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package risk is the minimal priority-scoring engine. It turns a small
// set of finding attributes into a factor-decomposed 0..100 priority
// score. The decomposition is the point: every score is explainable as a
// list of named contributions, so the assessment UI can justify a
// finding's priority without any model or AI.
//
// Scoring is pure and deterministic — same input, same output — so a
// finding's score is stable across runs and reproducible in tests.
package risk

import "strings"

// Factor is one named contribution to a priority score.
type Factor struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

// Scored is the result of scoring: the capped total plus the factors
// that produced it.
type Scored struct {
	Score   int      `json:"score"`
	Factors []Factor `json:"factors"`
}

// Input is the minimal attribute set the scorer reads. It is deliberately
// flat — the assess action fills it from the account row and the
// finding's severity/kind.
type Input struct {
	Severity   string // finding/policy severity: critical|high|medium|low
	Kind       string // finding kind, e.g. terminated_access
	Privileged bool
	MFAEnabled bool
	Active     bool
}

const maxScore = 100

// Score computes the factor-decomposed priority for one finding.
func Score(in Input) Scored {
	factors := make([]Factor, 0, 5)
	add := func(name string, pts int) {
		if pts != 0 {
			factors = append(factors, Factor{Name: name, Points: pts})
		}
	}

	switch strings.ToLower(in.Severity) {
	case "critical":
		add("severity:critical", 40)
	case "high":
		add("severity:high", 25)
	case "medium":
		add("severity:medium", 12)
	default:
		add("severity:low", 5)
	}

	if in.Privileged {
		add("privileged_account", 20)
	}
	if in.Privileged && !in.MFAEnabled {
		add("privileged_without_mfa", 15)
	}
	if !in.Active {
		add("inactive_account_with_access", 18)
	}

	// Lifecycle kinds amplify: standing access on a departed or
	// unmanaged identity is the highest-signal posture risk.
	switch in.Kind {
	case "terminated_access":
		add("terminated_subject", 20)
	case "orphaned_access":
		add("orphaned_subject", 12)
	case "dormant_privileged_access":
		add("dormant_privilege", 15)
	}

	total := 0
	for _, f := range factors {
		total += f.Points
	}
	if total > maxScore {
		total = maxScore
	}
	return Scored{Score: total, Factors: factors}
}
