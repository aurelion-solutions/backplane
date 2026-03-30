// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /employments envelope.
type ListResponse struct {
	Items  []*Employment `json:"items"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// RegisterRoutes mounts the employments HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/employments", listHandler(svc))
	g.POST("/employments", createHandler(svc))
	g.POST("/employments/bulk", bulkHandler(svc))
	g.GET("/employments/:id", getHandler(svc))
	g.PATCH("/employments/:id", patchHandler(svc))
	g.POST("/employments/:id/end", endHandler(svc))
	g.GET("/employments/:id/attributes", listAttributesHandler(svc))
	g.POST("/employments/:id/attributes", addAttributeHandler(svc))
	g.DELETE("/employments/:id/attributes/:key", removeAttributeHandler(svc))

	// Person-scoped listings — answers "who is currently working as
	// what for this person" and the full lifecycle history.
	g.GET("/persons/:personID/employments", listByPersonHandler(svc))
	g.GET("/persons/:personID/employments/active", listActiveHandler(svc))
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
		e, err := svc.Create(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, ErrPersonNotFound), errors.Is(err, ErrOrgUnitNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, e)
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
		e, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, e)
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
		e, err := svc.Update(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrOrgUnitNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusOK, e)
	}
}

func endHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in EndPayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		e, err := svc.End(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrAlreadyEnded), errors.Is(err, ErrInvalidDates):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, e)
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
			attrs = []*EmploymentAttribute{}
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

func listByPersonHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		pid, err := uuid.Parse(c.Param("personID"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		out, err := svc.ListByPerson(c.Request().Context(), pid)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if out == nil {
			out = []*Employment{}
		}
		return c.JSON(http.StatusOK, out)
	}
}

func listActiveHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		pid, err := uuid.Parse(c.Param("personID"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		at := time.Now().UTC()
		if v := c.QueryParam("at"); v != "" {
			if parsed, err := time.Parse(time.RFC3339, v); err == nil {
				at = parsed.UTC()
			}
		}
		out, err := svc.ListActiveByPerson(c.Request().Context(), pid, at)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if out == nil {
			out = []*Employment{}
		}
		return c.JSON(http.StatusOK, out)
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
