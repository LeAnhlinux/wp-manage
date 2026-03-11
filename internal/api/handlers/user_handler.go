package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type UserHandler struct {
	svc *services.UserService
}

func NewUserHandler(svc *services.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Create(c *fiber.Ctx) error {
	var req models.CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body: " + err.Error()})
	}
	if req.Email == "" || req.Password == "" || req.Name == "" || req.Role == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "Email, password, name and role are required"})
	}
	if req.Role != "admin" && req.Role != "manager" && req.Role != "viewer" {
		return c.Status(400).JSON(models.APIResponse{Error: "Role must be admin, manager, or viewer"})
	}

	user, err := h.svc.Create(req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}

	return c.Status(201).JSON(models.APIResponse{
		Success: true,
		Message: "User created",
		Data:    user,
	})
}

func (h *UserHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))

	users, total, err := h.svc.List(page, perPage)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch users"})
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    users,
		Meta: models.PaginationMeta{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (h *UserHandler) Get(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	user, err := h.svc.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "User not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: user})
}

func (h *UserHandler) Update(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}

	// Validate role if provided
	if req.Role != nil {
		r := *req.Role
		if r != "admin" && r != "manager" && r != "viewer" {
			return c.Status(400).JSON(models.APIResponse{Error: "Role must be admin, manager, or viewer"})
		}
	}

	user, err := h.svc.Update(id, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "User updated", Data: user})
}

func (h *UserHandler) Delete(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	// Prevent self-deletion
	currentUserID := c.Locals("user_id").(uint64)
	if id == currentUserID {
		return c.Status(400).JSON(models.APIResponse{Error: "Cannot delete your own account"})
	}

	if err := h.svc.Delete(id); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "User deleted"})
}

func (h *UserHandler) AssignServer(c *fiber.Ctx) error {
	userID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	var req models.AssignServerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	if req.ServerID == 0 {
		return c.Status(400).JSON(models.APIResponse{Error: "server_id is required"})
	}

	if err := h.svc.AssignServer(userID, req.ServerID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Server assigned to user"})
}

func (h *UserHandler) UnassignServer(c *fiber.Ctx) error {
	userID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	serverID, _ := strconv.ParseUint(c.Params("serverId"), 10, 64)

	if err := h.svc.UnassignServer(userID, serverID); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Server unassigned from user"})
}

func (h *UserHandler) GetAssignedServers(c *fiber.Ctx) error {
	userID, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	servers, err := h.svc.GetAssignedServers(userID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to fetch assigned servers"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: servers})
}
