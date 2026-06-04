// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ListResponse is the GET …/projections envelope.
type ListResponse struct {
	Items []ProjectionSummary `json:"items"`
}

// RegisterRoutes mounts the read-only projection surface under the
// assessment-run path. Every endpoint recomputes from the run — nothing
// is written, so all are safe to prefetch / revalidate.
//
// The run id param is `:id` to match the sibling assessment-run routes
// (echo requires a consistent param name at the same path position).
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/policy-assessment-runs/:id/projections", listHandler(svc))
	g.GET("/policy-assessment-runs/:id/projections/:projection", coverageHandler(svc))
	g.GET("/policy-assessment-runs/:id/projections/:projection/controls/:controlID", controlHandler(svc))
	g.GET("/policy-assessment-runs/:id/projections/:projection/packet", packetHandler(svc))
}

func listHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		runID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		items, err := svc.Projections(c.Request().Context(), runID)
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, ListResponse{Items: items})
	}
}

func coverageHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		runID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		report, err := svc.Coverage(c.Request().Context(), runID, c.Param("projection"))
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, report)
	}
}

func controlHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		runID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		detail, err := svc.ControlDetail(c.Request().Context(), runID, c.Param("projection"), c.Param("controlID"))
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, detail)
	}
}

func packetHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		runID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		projection := c.Param("projection")
		packet, err := svc.Packet(c.Request().Context(), runID, projection)
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		if c.QueryParam("format") == "csv" {
			filename := fmt.Sprintf("%s-%s.csv", projection, runID)
			c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename="+filename)
			c.Response().Header().Set(echo.HeaderContentType, "text/csv")
			c.Response().WriteHeader(http.StatusOK)
			return packet.WriteCSV(c.Response())
		}
		return c.JSON(http.StatusOK, packet)
	}
}

// statusFor maps engine errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrRunNotFound),
		errors.Is(err, ErrProjectionNotFound),
		errors.Is(err, ErrControlNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
