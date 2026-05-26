// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// ListResponse is the GET /accounts envelope.
type ListResponse struct {
	Items  []*Account `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// RegisterRoutes mounts the read-only account surface on g. Reads go
// through the supplied bun.DB; the write path (Upsert / Set*State) stays
// internal to the pipeline actions and is intentionally not exposed here.
func RegisterRoutes(g *echo.Group, db *bun.DB, repo Repository, lookup Lookup) {
	g.GET("/accounts", listHandler(db, repo))
	g.GET("/accounts/:id", getHandler(db, lookup))
}

// listHandler returns a paginated account snapshot, optionally narrowed to
// one application via ?application_id=.
func listHandler(db *bun.DB, repo Repository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := ListFilter{
			Limit:  parseIntDefault(c.QueryParam("limit"), 100),
			Offset: parseIntDefault(c.QueryParam("offset"), 0),
		}
		if raw := c.QueryParam("application_id"); raw != "" {
			appID, err := uuid.Parse(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"detail": "invalid application_id"})
			}
			f.ApplicationID = &appID
		}
		f.Privileged = parseBoolParam(c.QueryParam("privileged"))
		f.MFAEnabled = parseBoolParam(c.QueryParam("mfa"))
		f.Assigned = parseBoolParam(c.QueryParam("assigned"))
		items, total, err := repo.List(c.Request().Context(), db, f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		if items == nil {
			items = []*Account{}
		}
		return c.JSON(http.StatusOK, ListResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
	}
}

// getHandler resolves one account by id (read-only).
func getHandler(db *bun.DB, lookup Lookup) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"detail": "invalid account id"})
		}
		acc, err := lookup.GetByID(c.Request().Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "account not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, acc)
	}
}

// parseBoolParam returns a pointer to the parsed bool, or nil when the
// query param is absent/unparseable (→ filter not applied).
func parseBoolParam(s string) *bool {
	if s == "" {
		return nil
	}
	if v, err := strconv.ParseBool(s); err == nil {
		return &v
	}
	return nil
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
