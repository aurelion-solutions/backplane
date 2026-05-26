// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package consent

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// AppListResponse is the GET /consented-applications envelope.
type AppListResponse struct {
	Items  []*ConsentedApplication `json:"items"`
	Total  int                     `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}

// GrantListResponse is the GET /consent-grants envelope.
type GrantListResponse struct {
	Items  []*ConsentGrant `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// RegisterRoutes mounts the read-only consent surface on g. Reads go
// through the supplied bun.DB; the write path (Upsert) stays internal to
// the connector/ingest actions and is intentionally not exposed here.
func RegisterRoutes(
	g *echo.Group,
	db *bun.DB,
	apps AppRepository,
	appLookup AppLookup,
	grants GrantRepository,
	grantLookup GrantLookup,
) {
	g.GET("/consented-applications", listAppHandler(db, apps))
	g.GET("/consented-applications/:id", getAppHandler(db, appLookup))
	g.GET("/consent-grants", listGrantHandler(db, grants))
	g.GET("/consent-grants/:id", getGrantHandler(db, grantLookup))
}

func listAppHandler(db *bun.DB, repo AppRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := AppListFilter{
			Origin:               c.QueryParam("origin"),
			ResolutionConfidence: c.QueryParam("resolution_confidence"),
			Limit:                parseIntDefault(c.QueryParam("limit"), 100),
			Offset:               parseIntDefault(c.QueryParam("offset"), 0),
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("resolved_principal_id")); err != nil {
			return badRequest(c, "invalid resolved_principal_id")
		} else if ok {
			f.ResolvedPrincipalID = id
		}
		f.VerifiedPublisher = parseBoolParam(c.QueryParam("verified_publisher"))
		f.Resolved = parseBoolParam(c.QueryParam("resolved"))
		items, total, err := repo.List(c.Request().Context(), db, f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		if items == nil {
			items = []*ConsentedApplication{}
		}
		return c.JSON(http.StatusOK, AppListResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
	}
}

func getAppHandler(db *bun.DB, lookup AppLookup) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return badRequest(c, "invalid application id")
		}
		a, err := lookup.GetByID(c.Request().Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "consented application not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, a)
	}
}

func listGrantHandler(db *bun.DB, repo GrantRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := GrantListFilter{
			GrantType: c.QueryParam("grant_type"),
			Limit:     parseIntDefault(c.QueryParam("limit"), 100),
			Offset:    parseIntDefault(c.QueryParam("offset"), 0),
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("consented_application_id")); err != nil {
			return badRequest(c, "invalid consented_application_id")
		} else if ok {
			f.ConsentedApplicationID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("consenting_principal_id")); err != nil {
			return badRequest(c, "invalid consenting_principal_id")
		} else if ok {
			f.ConsentingPrincipalID = id
		}
		f.Active = parseBoolParam(c.QueryParam("active"))
		f.Owned = parseBoolParam(c.QueryParam("owned"))
		items, total, err := repo.List(c.Request().Context(), db, f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		if items == nil {
			items = []*ConsentGrant{}
		}
		return c.JSON(http.StatusOK, GrantListResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
	}
}

func getGrantHandler(db *bun.DB, lookup GrantLookup) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return badRequest(c, "invalid grant id")
		}
		g, err := lookup.GetByID(c.Request().Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "consent grant not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, g)
	}
}

func badRequest(c echo.Context, detail string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"detail": detail})
}

// parseUUIDParam parses an optional uuid query param. Returns
// (nil, false, nil) when absent, (id, true, nil) when valid, and a
// non-nil error when present-but-unparseable.
func parseUUIDParam(s string) (*uuid.UUID, bool, error) {
	if s == "" {
		return nil, false, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, false, err
	}
	return &id, true, nil
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
