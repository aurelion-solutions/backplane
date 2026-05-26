// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secrets

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// PlainListResponse is the GET /secrets/plain envelope.
type PlainListResponse struct {
	Items  []*SecretPlain `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// CertListResponse is the GET /secrets/certificates envelope.
type CertListResponse struct {
	Items  []*SecretCertificate `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// RegisterRoutes mounts the read-only secret surface on g. Reads go
// through the supplied bun.DB; the write path (Upsert) stays internal to
// the connector/ingest actions and is intentionally not exposed here.
func RegisterRoutes(
	g *echo.Group,
	db *bun.DB,
	plain PlainRepository,
	plainLookup PlainLookup,
	cert CertRepository,
	certLookup CertLookup,
) {
	g.GET("/secrets/plain", listPlainHandler(db, plain))
	g.GET("/secrets/plain/:id", getPlainHandler(db, plainLookup))
	g.GET("/secrets/certificates", listCertHandler(db, cert))
	g.GET("/secrets/certificates/:id", getCertHandler(db, certLookup))
}

func listPlainHandler(db *bun.DB, repo PlainRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := PlainListFilter{
			Type:   c.QueryParam("type"),
			Limit:  parseIntDefault(c.QueryParam("limit"), 100),
			Offset: parseIntDefault(c.QueryParam("offset"), 0),
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("target_application_id")); err != nil {
			return badRequest(c, "invalid target_application_id")
		} else if ok {
			f.TargetApplicationID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("found_in_application_id")); err != nil {
			return badRequest(c, "invalid found_in_application_id")
		} else if ok {
			f.FoundInApplicationID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("account_id")); err != nil {
			return badRequest(c, "invalid account_id")
		} else if ok {
			f.AccountID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("principal_id")); err != nil {
			return badRequest(c, "invalid principal_id")
		} else if ok {
			f.PrincipalID = id
		}
		f.Privileged = parseBoolParam(c.QueryParam("privileged"))
		f.Linked = parseBoolParam(c.QueryParam("linked"))
		items, total, err := repo.List(c.Request().Context(), db, f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		if items == nil {
			items = []*SecretPlain{}
		}
		return c.JSON(http.StatusOK, PlainListResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
	}
}

func getPlainHandler(db *bun.DB, lookup PlainLookup) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return badRequest(c, "invalid secret id")
		}
		s, err := lookup.GetByID(c.Request().Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "secret not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, s)
	}
}

func listCertHandler(db *bun.DB, repo CertRepository) echo.HandlerFunc {
	return func(c echo.Context) error {
		f := CertListFilter{
			Format: c.QueryParam("format"),
			Limit:  parseIntDefault(c.QueryParam("limit"), 100),
			Offset: parseIntDefault(c.QueryParam("offset"), 0),
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("target_application_id")); err != nil {
			return badRequest(c, "invalid target_application_id")
		} else if ok {
			f.TargetApplicationID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("found_in_application_id")); err != nil {
			return badRequest(c, "invalid found_in_application_id")
		} else if ok {
			f.FoundInApplicationID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("account_id")); err != nil {
			return badRequest(c, "invalid account_id")
		} else if ok {
			f.AccountID = id
		}
		if id, ok, err := parseUUIDParam(c.QueryParam("principal_id")); err != nil {
			return badRequest(c, "invalid principal_id")
		} else if ok {
			f.PrincipalID = id
		}
		f.Privileged = parseBoolParam(c.QueryParam("privileged"))
		f.Linked = parseBoolParam(c.QueryParam("linked"))
		items, total, err := repo.List(c.Request().Context(), db, f)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		if items == nil {
			items = []*SecretCertificate{}
		}
		return c.JSON(http.StatusOK, CertListResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
	}
}

func getCertHandler(db *bun.DB, lookup CertLookup) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return badRequest(c, "invalid certificate id")
		}
		cert, err := lookup.GetByID(c.Request().Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "certificate not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, cert)
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
