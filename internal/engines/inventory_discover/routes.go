// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the discover HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/discover/runs", fetchHandler(svc))
	g.GET("/discover/runs", listHandler(svc))
	g.GET("/discover/runs/:id", getHandler(svc))
}

func fetchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in FetchPayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := svc.Fetch(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidEnvelope):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			case errors.Is(err, ErrDispatch):
				return c.JSON(http.StatusBadGateway, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusAccepted, row)
	}
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

func getHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := svc.Get(c.Request().Context(), id)
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
