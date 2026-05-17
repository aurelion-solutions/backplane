// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"strings"
	"text/template"
)

// templateFuncs is the FuncMap exposed inside descriptor templates
// (`{{ .Principal.Firstname | lower }}`).
//
// Each function here is pure and stateless. Equivalent helpers exist
// in the post-template transforms pipeline (transforms.go); use
// whichever reads better in the YAML — inside the template when the
// value is naturally piped, in `transforms:` when the chain is long
// enough to warrant a separate clause.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}
}
