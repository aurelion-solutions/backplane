// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the employee-records HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	// Records
	g.GET("/employee-records", listRecordsHandler(svc))
	g.POST("/employee-records", createRecordHandler(svc))
	g.POST("/employee-records/bulk", bulkHandler(svc))
	g.GET("/employee-records/:id", getRecordHandler(svc))

	// Attributes
	g.GET("/employee-records/:id/attributes", listAttributesHandler(svc))
	g.POST("/employee-records/:id/attributes", addAttributeHandler(svc))
	g.DELETE("/employee-records/:id/attributes/:key", removeAttributeHandler(svc))

	// Match
	g.GET("/employee-records/:id/match", getMatchHandler(svc))
	g.PUT("/employee-records/:id/match", setMatchHandler(svc))
	g.DELETE("/employee-records/:id/match", clearMatchHandler(svc))
	g.POST("/employee-records/:id/resolve", resolveHandler(svc))

	// Mappings (per-application)
	g.GET("/applications/:appID/employee-record-mappings", listMappingsHandler(svc))
	g.POST("/applications/:appID/employee-record-mappings", createMappingHandler(svc))
	g.DELETE("/employee-record-mappings/:id", deleteMappingHandler(svc))

	// All matches at once — clients use this to enrich a record list
	// with (person, employment) without paying for N+1 lookups.
	g.GET("/employee-record-matches", listMatchesHandler(svc))
}

func listMatchesHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		items, err := svc.ListMatches(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if items == nil {
			items = []*EmployeeRecordMatch{}
		}
		return c.JSON(http.StatusOK, items)
	}
}

func listRecordsHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		items, err := svc.ListRecords(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, items)
	}
}

func createRecordHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in CreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := svc.CreateRecord(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, ErrApplicationNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			case errors.Is(err, ErrDuplicate):
				return c.JSON(http.StatusConflict, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, row)
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

func getRecordHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := svc.GetRecord(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, row)
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
			attrs = []*EmployeeRecordAttribute{}
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

func getMatchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		m, err := svc.GetMatch(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if m == nil {
			return c.NoContent(http.StatusNoContent)
		}
		return c.JSON(http.StatusOK, m)
	}
}

func setMatchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in MatchCreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		m, err := svc.SetMatch(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrPersonNotFound), errors.Is(err, ErrEmploymentNotFound):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusOK, m)
	}
}

func clearMatchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if err := svc.ClearMatch(c.Request().Context(), id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func resolveHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		res, err := svc.ResolveAndPersist(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, res)
	}
}

func listMappingsHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		appID, err := uuid.Parse(c.Param("appID"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		out, err := svc.ListMappings(c.Request().Context(), appID)
		if err != nil {
			if errors.Is(err, ErrApplicationNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		if out == nil {
			out = []*EmployeeProviderAttributeMapping{}
		}
		return c.JSON(http.StatusOK, out)
	}
}

func createMappingHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		appID, err := uuid.Parse(c.Param("appID"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in MappingCreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		m, err := svc.CreateMapping(c.Request().Context(), appID, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrApplicationNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrMappingDuplicate):
				return c.JSON(http.StatusConflict, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, m)
	}
}

func deleteMappingHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if err := svc.DeleteMapping(c.Request().Context(), id); err != nil {
			if errors.Is(err, ErrMappingNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
