// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

import (
	"errors"
	"net/http"
	"time"

	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/labstack/echo/v4"
)

// Deps bundles what the transport needs from the host process.
type Deps struct {
	Store      *policy_assessment.Store
	Dispatcher *policy_assessment.Dispatcher
}

// RegisterRoutes mounts the AuthZen surface on g.
func RegisterRoutes(g *echo.Group, deps Deps) {
	g.POST("/access/v1/evaluation", evaluateHandler(deps))
}

func evaluateHandler(deps Deps) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req Request
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if err := req.Validate(); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		facets := deriveFacets(req)
		entries := deps.Store.SelectByFacets(facets)

		facts := buildFacts(req)
		outs := make([]perPolicyOutput, 0, len(entries))
		for _, e := range entries {
			policyID := e.CartridgeRef + "/" + e.Manifest.RuleID
			if !deps.Dispatcher.Has(e.Manifest.Mechanism) {
				// Mechanism not registered in this runtime — silently
				// skip; another caller (worker scan) may host it.
				continue
			}
			out, err := deps.Dispatcher.Evaluate(c.Request().Context(),
				policy_assessment.Request{
					Mechanism:    e.Manifest.Mechanism,
					PolicyID:     policyID,
					CartridgeRef: e.CartridgeRef,
					BasePath:     e.Manifest.BasePath,
					Body:         e.Manifest.Body,
					Facts:        facts,
				})
			outs = append(outs, perPolicyOutput{
				PolicyID:  policyID,
				Cartridge: e.CartridgeRef,
				Output:    out,
				Err:       skipDispatchErr(err),
			})
		}
		resp := aggregate(outs)
		return c.JSON(http.StatusOK, resp)
	}
}

// buildFacts shapes the canonical Facts the engine consumes from the
// AuthZen envelope. The AuthZen wire shape calls the actor "subject" —
// Aurelion's kernel contract calls it "principal", so we translate
// `req.Subject` → `Facts.Principal` here. The caller (IdP / gateway)
// is responsible for putting the relevant signals in `context.*` and
// any extra entity records in `context.entities`.
func buildFacts(req Request) policy_assessment.Facts {
	principal := &policy_assessment.PrincipalFacts{
		ID:   req.Subject.ID,
		Kind: req.Subject.Type,
	}
	for k, v := range req.Subject.Properties {
		switch k {
		case "status":
			if s, ok := v.(string); ok {
				principal.Status = s
			}
		case "org_unit":
			if s, ok := v.(string); ok {
				principal.OrgUnit = s
			}
		case "tenant_id":
			if s, ok := v.(string); ok {
				principal.TenantID = s
			}
		case "mfa_enabled":
			if b, ok := v.(bool); ok {
				principal.MFAEnabled = &b
			}
		case "email_verified":
			if b, ok := v.(bool); ok {
				principal.EmailVerified = &b
			}
		default:
			if principal.Attributes == nil {
				principal.Attributes = map[string]any{}
			}
			principal.Attributes[k] = v
		}
	}

	facts := policy_assessment.Facts{
		Principal: principal,
		Action:    req.Action.Name,
		Resource:  &policy_assessment.Resource{Type: req.Resource.Type, ID: req.Resource.ID, Properties: req.Resource.Properties},
		Now:       time.Now().UTC(),
	}

	if len(req.Context) > 0 {
		ctxFacts := &policy_assessment.ContextFacts{Extra: map[string]any{}}
		for k, v := range req.Context {
			switch k {
			case "entities":
				// caller-supplied entity records → Facts.Entities
				facts.Entities = parseEntities(v)
			case "transport":
				if s, ok := v.(string); ok {
					ctxFacts.Transport = s
				}
			case "country":
				if s, ok := v.(string); ok {
					ctxFacts.Country = s
				}
			case "ip":
				if s, ok := v.(string); ok {
					ctxFacts.IP = s
				}
			default:
				ctxFacts.Extra[k] = v
			}
		}
		if ctxFacts.Transport != "" || ctxFacts.Country != "" || ctxFacts.IP != "" || len(ctxFacts.Extra) > 0 {
			facts.Context = ctxFacts
		}
	}

	return facts
}

func parseEntities(v any) []policy_assessment.EntityRecord {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]policy_assessment.EntityRecord, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		uidM, _ := m["uid"].(map[string]any)
		uidType, _ := uidM["type"].(string)
		uidID, _ := uidM["id"].(string)
		if uidType == "" || uidID == "" {
			continue
		}
		rec := policy_assessment.EntityRecord{
			UID:   policy_assessment.EntityUID{Type: uidType, ID: uidID},
			Attrs: stringMap(m["attrs"]),
		}
		if parents, ok := m["parents"].([]any); ok {
			for _, p := range parents {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				pt, _ := pm["type"].(string)
				pi, _ := pm["id"].(string)
				if pt == "" || pi == "" {
					continue
				}
				rec.Parents = append(rec.Parents, policy_assessment.EntityUID{Type: pt, ID: pi})
			}
		}
		out = append(out, rec)
	}
	return out
}

func stringMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// skipDispatchErr drops the harmless ErrUnknownMechanism error that
// surfaces when a policy references a mechanism not registered in this
// process; everything else is preserved.
func skipDispatchErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, policy_assessment.ErrUnknownMechanism) {
		return nil
	}
	return err
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
