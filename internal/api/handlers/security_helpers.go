package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type revealSecretRequest struct {
	Password string `json:"password"`
}

func currentAuthSessionFromRequest(c *fiber.Ctx, service *auth.Service) (auth.Session, bool) {
	if session, ok := c.Locals("auth_session").(auth.Session); ok {
		return session, true
	}
	if service == nil {
		return auth.Session{}, false
	}
	return service.Session(c.Cookies(service.CookieName()))
}

func requireRevealAuthorization(c *fiber.Ctx, service *auth.Service) (auth.Session, error) {
	session, ok := currentAuthSessionFromRequest(c, service)
	if !ok {
		return auth.Session{}, fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	var req revealSecretRequest
	if err := c.BodyParser(&req); err != nil {
		return auth.Session{}, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Password) == "" {
		return auth.Session{}, fiber.NewError(fiber.StatusBadRequest, "password is required")
	}

	if _, err := service.Authenticate(c.UserContext(), session.Username, req.Password); err != nil {
		if err == auth.ErrInvalidCredentials {
			return auth.Session{}, fiber.NewError(fiber.StatusUnauthorized, "password is incorrect")
		}
		return auth.Session{}, err
	}

	return session, nil
}

func writeAuditLog(ctx *fiber.Ctx, store *query.AuditLogStore, session auth.Session, action string, resourceType string, resourceID string, details any) error {
	if store == nil {
		return nil
	}

	actorID := session.UserID
	resourceTypeCopy := strings.TrimSpace(resourceType)
	resourceIDCopy := strings.TrimSpace(resourceID)

	entry := models.AuditLog{
		ActorType:    "user",
		ActorID:      &actorID,
		Action:       strings.TrimSpace(action),
		ResourceType: &resourceTypeCopy,
		ResourceID:   &resourceIDCopy,
		Details:      query.MarshalAuditDetails(details),
	}
	if entry.Action == "" {
		return fmt.Errorf("audit action is required")
	}
	return store.Create(ctx.Context(), entry)
}
