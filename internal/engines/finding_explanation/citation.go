// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import "regexp"

// labelToken matches a bracketed citation label like [F], [P1], [E12].
var labelToken = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9]*)\]`)

// validateCitations is the deterministic boundary enforcement: it pulls
// every bracketed label out of the narrative and keeps only those that
// match a reference actually placed in the model's context. Labels the
// model invented (not in refs) are dropped — they never become
// citations. Returns the validated citations (deduplicated, in first-
// appearance order) and the list of stray labels for logging.
func validateCitations(narrative string, refs []Reference) (cited []Citation, stray []string) {
	byLabel := make(map[string]Reference, len(refs))
	for _, r := range refs {
		byLabel[r.Label] = r
	}

	seen := make(map[string]bool)
	for _, m := range labelToken.FindAllStringSubmatch(narrative, -1) {
		label := m[1]
		if seen[label] {
			continue
		}
		seen[label] = true
		if r, ok := byLabel[label]; ok {
			cited = append(cited, Citation{Label: r.Label, Kind: r.Kind, ID: r.ID})
		} else {
			stray = append(stray, label)
		}
	}
	return cited, stray
}
