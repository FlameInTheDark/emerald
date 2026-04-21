package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type UserHandler struct {
	store       *query.UserStore
	authService *auth.Service
	trustProxy  bool
}

type UserHandlerOptions struct {
	TrustProxy bool
}

func NewUserHandler(store *query.UserStore, authService *auth.Service, opts ...UserHandlerOptions) *UserHandler {
	handler := &UserHandler{store: store, authService: authService}
	if len(opts) > 0 {
		handler.trustProxy = opts[0].TrustProxy
	}
	return handler
}

func (h *UserHandler) List(c *fiber.Ctx) error {
	users, err := h.store.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if users == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(users)
}

func (h *UserHandler) Create(c *fiber.Ctx) error {
	session, ok := h.currentSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	if !session.IsSuperAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the super-admin can create users"})
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "username and password are required"})
	}

	existing, err := h.store.GetByUsername(c.Context(), req.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if existing != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already exists"})
	}

	user := models.User{
		Username: req.Username,
		Password: req.Password,
	}
	if err := h.store.Create(c.Context(), &user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	user.Password = ""
	return c.Status(fiber.StatusCreated).JSON(user)
}

func (h *UserHandler) Delete(c *fiber.Ctx) error {
	session, ok := h.currentSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
	if !session.IsSuperAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the super-admin can delete users"})
	}
	if session.UserID == c.Params("id") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "you cannot delete your own account"})
	}

	target, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if target == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	if target.IsSuperAdmin {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "the bootstrap super-admin cannot be removed"})
	}

	if err := h.store.Delete(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if h.authService != nil {
		h.authService.RevokeUserSessions(c.Params("id"))
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *UserHandler) ChangePassword(c *fiber.Ctx) error {
	if h.authService == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "authentication service is not configured"})
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "current password and new password are required"})
	}
	if req.CurrentPassword == req.NewPassword {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new password must be different"})
	}

	sessionToken := c.Cookies(h.authService.CookieName())
	session, ok := h.authService.Session(sessionToken)
	if !ok {
		clearAuthCookie(c, h.authService.CookieName(), h.trustProxy)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	user, err := h.authService.Authenticate(c.UserContext(), session.Username, req.CurrentPassword)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current password is incorrect"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to validate credentials"})
	}

	if err := h.store.UpdatePassword(c.Context(), user.ID, req.NewPassword); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update password"})
	}

	h.authService.RevokeUserSessions(user.ID)

	newToken, newSession, err := h.authService.Login(c.UserContext(), user.Username, req.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to refresh session"})
	}

	setAuthCookie(c, h.authService.CookieName(), newToken, newSession.ExpiresAt, h.trustProxy)
	return c.JSON(toAuthSessionResponse(newSession))
}

func (h *UserHandler) currentSession(c *fiber.Ctx) (auth.Session, bool) {
	if session, ok := c.Locals("auth_session").(auth.Session); ok {
		return session, true
	}
	if h.authService == nil {
		return auth.Session{}, false
	}
	return h.authService.Session(c.Cookies(h.authService.CookieName()))
}
