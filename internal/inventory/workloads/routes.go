// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET /workloads envelope.
type ListResponse struct {
	Items  []*Workload `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// RegisterRoutes mounts the workloads HTTP surface on g. Note: no
// /expire endpoint — locking moved to /principals/:id/lock.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/workloads", listHandler(svc))
	g.POST("/workloads", createHandler(svc))
	g.POST("/workloads/bulk", bulkHandler(svc))
	g.GET("/workloads/:id", getHandler(svc))
	g.PATCH("/workloads/:id", patchHandler(svc))
	g.GET("/workloads/:id/attributes", listAttributesHandler(svc))
	g.POST("/workloads/:id/attributes", addAttributeHandler(svc))
	g.DELETE("/workloads/:id/attributes/:key", removeAttributeHandler(svc))
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
		w, err := svc.Create(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, ErrExternalIDAlreadyExists):
				return c.JSON(http.StatusConflict, errorBody(err))
			case errors.Is(err, ErrOwnerNotFound), errors.Is(err, ErrApplicationNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, w)
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
		w, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, w)
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
		w, err := svc.Update(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrOwnerNotFound), errors.Is(err, ErrApplicationNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusOK, w)
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
			attrs = []*WorkloadAttribute{}
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
