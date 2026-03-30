// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /persons envelope.
type ListResponse struct {
	Items  []*Person `json:"items"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// RegisterRoutes mounts the persons HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/persons", listHandler(svc))
	g.POST("/persons", createHandler(svc))
	g.POST("/persons/bulk", bulkHandler(svc))
	g.GET("/persons/:id", getHandler(svc))
	g.GET("/persons/:id/attributes", listAttributesHandler(svc))
	g.POST("/persons/:id/attributes", addAttributeHandler(svc))
	g.DELETE("/persons/:id/attributes/:key", removeAttributeHandler(svc))
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
		p, err := svc.Create(c.Request().Context(), in)
		if err != nil {
			if errors.Is(err, ErrExternalIDAlreadyExists) {
				return c.JSON(http.StatusConflict, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, p)
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
			switch {
			case errors.Is(err, ErrBulkEmpty), errors.Is(err, ErrBulkTooLarge):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
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
		p, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, p)
	}
}

func listAttributesHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		attrs, err := svc.ListAttributes(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if attrs == nil {
			attrs = []*PersonAttribute{}
		}
		return c.JSON(http.StatusOK, attrs)
	}
}

func addAttributeHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in AttributeCreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		a, err := svc.AddAttribute(c.Request().Context(), id, in)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, a)
	}
}

func removeAttributeHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		key := c.Param("key")
		if err := svc.RemoveAttribute(c.Request().Context(), id, key); err != nil {
			switch {
			case errors.Is(err, ErrNotFound), errors.Is(err, ErrAttributeNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
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
