// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package findings

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /findings envelope.
type ListResponse struct {
	Items  []*Finding `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// RegisterRoutes mounts the read-only findings HTTP surface on g.
//
// No POST / PATCH / DELETE: findings are written by the
// policy-assessment action when a policy fires. Status transitions
// (open → acknowledged → resolved / mitigated) will arrive through
// dedicated action endpoints owned by their own slice.
func RegisterRoutes(g *echo.Group, repo Repository) {
	g.GET("/findings", listHandler(repo))
	g.GET("/findings/:id", getHandler(repo))
}

func listHandler(repo Repository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := ListFilter{
			Kind:         c.QueryParam("kind"),
			ExcludeKind:  c.QueryParam("exclude_kind"),
			TargetType:   c.QueryParam("target_type"),
			Status:       c.QueryParam("status"),
			Severity:     c.QueryParam("severity"),
			Source:       c.QueryParam("source"),
			CartridgeRef: c.QueryParam("cartridge"),
			Owner:        c.QueryParam("owner"),
			Limit:        parseIntDefault(c.QueryParam("limit"), 100),
			Offset:       parseIntDefault(c.QueryParam("offset"), 0),
		}
		for _, p := range []struct {
			q   string
			dst **uuid.UUID
		}{
			{"principal_id", &f.PrincipalID},
			{"target_id", &f.TargetID},
			{"application_id", &f.ApplicationID},
			{"policy_id", &f.PolicyID},
			{"assessment_run_id", &f.AssessmentRunID},
			{"last_seen_run_id", &f.LastSeenRunID},
		} {
			if s := c.QueryParam(p.q); s != "" {
				id, err := uuid.Parse(s)
				if err != nil {
					return c.JSON(http.StatusBadRequest, errorBody(err))
				}
				*p.dst = &id
			}
		}
		items, total, err := repo.List(c.Request().Context(), f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, ListResponse{
			Items: items, Total: total, Limit: f.Limit, Offset: f.Offset,
		})
	}
}

func getHandler(repo Repository) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := repo.GetByID(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, row)
	}
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
