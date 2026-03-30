// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /org-units envelope.
type ListResponse struct {
	Items  []*OrgUnit `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// RegisterRoutes mounts the org-units HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/org-units", listHandler(svc))
	g.POST("/org-units", createHandler(svc))
	g.POST("/org-units/bulk", bulkHandler(svc))
	g.GET("/org-units/:id", getHandler(svc))
	g.PATCH("/org-units/:id", patchHandler(svc))
	g.DELETE("/org-units/:id", deleteHandler(svc))
}

func listHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit := parseIntDefault(c.QueryParam("limit"), 100)
		offset := parseIntDefault(c.QueryParam("offset"), 0)
		items, total, err := svc.List(c.Request().Context(), limit, offset)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, ListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
	}
}

func createHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in CreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		u, err := svc.Create(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, ErrExternalIDAlreadyExists):
				return c.JSON(http.StatusConflict, errorBody(err))
			case errors.Is(err, ErrParentNotFound), errors.Is(err, ErrParentInternal):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, u)
	}
}

func bulkHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in BulkPayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		res, err := svc.BulkUpsert(c.Request().Context(), in)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusOK, res)
	}
}

func getHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		u, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, u)
	}
}

func patchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in PatchPayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		u, err := svc.Update(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrCannotDeleteInternal):
				return c.JSON(http.StatusForbidden, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusOK, u)
	}
}

func deleteHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if err := svc.Delete(c.Request().Context(), id); err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrCannotDeleteInternal):
				return c.JSON(http.StatusForbidden, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.NoContent(http.StatusNoContent)
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
