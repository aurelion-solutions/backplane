// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /policy-evaluation-outcomes envelope.
type ListResponse struct {
	Items  []*PolicyEvaluationOutcome `json:"items"`
	Total  int                        `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

// RegisterRoutes mounts the read-only PEO surface on g. Outcomes are
// written by the worker policy-assessment action, not by clients.
func RegisterRoutes(g *echo.Group, repo Repository) {
	g.GET("/policy-evaluation-outcomes", listHandler(repo))
	g.GET("/policy-evaluation-outcomes/:id", getHandler(repo))
}

func listHandler(repo Repository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := ListFilter{
			Outcome:     c.QueryParam("outcome"),
			CartridgeID: c.QueryParam("cartridge_id"),
			TargetType:  c.QueryParam("target_type"),
			Limit:       parseIntDefault(c.QueryParam("limit"), 100),
			Offset:      parseIntDefault(c.QueryParam("offset"), 0),
		}
		if s := c.QueryParam("assessment_run_id"); s != "" {
			id, err := uuid.Parse(s)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			f.AssessmentRunID = &id
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
