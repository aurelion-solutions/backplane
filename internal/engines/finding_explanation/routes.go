// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the explanation surface.
//
//	POST /findings/:id/explanations          generate (or reuse) an explanation
//	GET  /findings/:id/explanations/latest    the newest explanation for a finding
//	GET  /explanation-jobs/:id                one explanation by its id
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/findings/:id/explanations", explainHandler(svc))
	g.GET("/findings/:id/explanations/latest", latestHandler(svc))
	g.GET("/explanation-jobs/:id", getHandler(svc))
}

func explainHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		findingID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var req ExplainRequest
		if c.Request().ContentLength != 0 {
			if bErr := c.Bind(&req); bErr != nil {
				return c.JSON(http.StatusBadRequest, errorBody(bErr))
			}
		}
		view, err := svc.Explain(c.Request().Context(), findingID, req)
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, view)
	}
}

func latestHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		findingID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		view, err := svc.Latest(c.Request().Context(), findingID)
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, view)
	}
}

func getHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		view, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			return c.JSON(statusFor(err), errorBody(err))
		}
		return c.JSON(http.StatusOK, view)
	}
}

// statusFor maps engine errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrFindingNotFound), errors.Is(err, ErrExplanationNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrGenerationFailed):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
