// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"

	"github.com/google/uuid"
)

// PacketControl is one control's full evidence bundle inside a packet:
// the control definition, its coverage state, the supporting findings,
// and the blind spots.
type PacketControl struct {
	Control    Control      `json:"control"`
	State      string       `json:"state"`
	Violations []FindingRef `json:"violations"`
	Gaps       []GapRef     `json:"gaps"`
}

// Packet is the exportable external-evidence packet for one projection
// over one assessment run. It is a pure serialisation of the computed
// coverage — it asserts no audit opinion and persists nothing.
type Packet struct {
	Projection     string          `json:"projection"`
	Name           string          `json:"name"`
	Type           string          `json:"type,omitempty"`
	CriteriaSource string          `json:"criteria_source,omitempty"`
	Disclaimer     string          `json:"disclaimer,omitempty"`
	AssessmentRun  uuid.UUID       `json:"assessment_run_id"`
	Period         Period          `json:"period"`
	Summary        map[string]int  `json:"summary"`
	Controls       []PacketControl `json:"controls"`
}

// Packet builds the full evidence packet for a projection over a run.
func (s *Service) Packet(ctx context.Context, runID uuid.UUID, projection string) (Packet, error) {
	def, err := loadDefinition(s.cartridges, projection)
	if err != nil {
		return Packet{}, err
	}
	rc, err := s.loadRunContext(ctx, runID)
	if err != nil {
		return Packet{}, err
	}
	controls, summary := computeCoverage(def, rc.findingsByKind, rc.outcomes, rc.ruleKind)
	stateByID := make(map[string]string, len(controls))
	for _, cc := range controls {
		stateByID[cc.ControlID] = cc.State
	}

	packetControls := make([]PacketControl, 0, len(def.Controls))
	for _, c := range def.Controls {
		violations, err := s.violationRefs(ctx, runID, c)
		if err != nil {
			return Packet{}, err
		}
		packetControls = append(packetControls, PacketControl{
			Control:    c,
			State:      stateByID[c.ControlID],
			Violations: violations,
			Gaps:       s.gapRefs(c, rc.outcomes, rc.ruleKind),
		})
	}

	return Packet{
		Projection:     def.Projection,
		Name:           def.Name,
		Type:           def.Type,
		CriteriaSource: def.CriteriaSource,
		Disclaimer:     def.Disclaimer,
		AssessmentRun:  runID,
		Period:         rc.period,
		Summary:        summary,
		Controls:       packetControls,
	}, nil
}

// WriteCSV serialises a packet's per-control coverage as CSV — the form
// an auditor's working papers most often want. One row per control with
// its state and evidence counts.
func (p Packet) WriteCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"control_id", "title", "category", "state", "violations", "gaps"}); err != nil {
		return err
	}
	for _, pc := range p.Controls {
		row := []string{
			pc.Control.ControlID,
			pc.Control.Title,
			pc.Control.Category,
			pc.State,
			strconv.Itoa(len(pc.Violations)),
			strconv.Itoa(len(pc.Gaps)),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
