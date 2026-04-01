package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type TeamHandler struct {
	svc *services.TeamService
}

func NewTeamHandler(svc *services.TeamService) *TeamHandler {
	return &TeamHandler{svc: svc}
}

// Create creates a new team
func (h *TeamHandler) Create(c *fiber.Ctx) error {
	var req models.CreateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Team name is required")
	}

	userID, _ := c.Locals("user_id").(uint64)
	team, err := h.svc.Create(userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(models.APIResponse{Success: true, Data: team})
}

// List returns teams visible to the current user
func (h *TeamHandler) List(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	pg := parsePage(c)

	teams, total, err := h.svc.List(userID, role, pg.Page, pg.PerPage)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	totalPages := calcTotalPages(total, pg.PerPage)
	return c.JSON(models.PaginatedResponse{
		Success: true,
		Data:    teams,
		Meta:    models.PaginationMeta{Page: pg.Page, PerPage: pg.PerPage, Total: total, TotalPages: totalPages},
	})
}

// Get returns a single team
func (h *TeamHandler) Get(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}

	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if !h.svc.HasTeamAccess(userID, role, teamID) {
		return fiber.NewError(fiber.StatusForbidden, "No access to this team")
	}

	team, err := h.svc.GetByID(teamID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: team})
}

// Update updates a team
func (h *TeamHandler) Update(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}

	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if !h.svc.IsTeamAdmin(userID, role, teamID) {
		return fiber.NewError(fiber.StatusForbidden, "Only team admins can update team")
	}

	var req models.UpdateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	team, err := h.svc.Update(teamID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: team})
}

// Delete deletes a team
func (h *TeamHandler) Delete(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}

	if err := h.svc.Delete(teamID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Team deleted"})
}

// AddMember adds a member to a team
func (h *TeamHandler) AddMember(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}

	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if !h.svc.IsTeamAdmin(userID, role, teamID) {
		return fiber.NewError(fiber.StatusForbidden, "Only team admins can add members")
	}

	var req models.AddTeamMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	member, err := h.svc.AddMember(teamID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(models.APIResponse{Success: true, Data: member})
}

// UpdateMember updates a member's role in a team
func (h *TeamHandler) UpdateMember(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}
	memberUserID, err := strconv.ParseUint(c.Params("userId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if !h.svc.IsTeamAdmin(userID, role, teamID) {
		return fiber.NewError(fiber.StatusForbidden, "Only team admins can update members")
	}

	var req models.UpdateTeamMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	member, err := h.svc.UpdateMember(teamID, memberUserID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Data: member})
}

// RemoveMember removes a member from a team
func (h *TeamHandler) RemoveMember(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}
	memberUserID, err := strconv.ParseUint(c.Params("userId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}

	userID, _ := c.Locals("user_id").(uint64)
	role, _ := c.Locals("role").(string)

	if !h.svc.IsTeamAdmin(userID, role, teamID) {
		return fiber.NewError(fiber.StatusForbidden, "Only team admins can remove members")
	}

	if err := h.svc.RemoveMember(teamID, memberUserID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Member removed"})
}

// AssignServer assigns a server to a team
func (h *TeamHandler) AssignServer(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}

	var req models.AssignTeamServerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	ts, err := h.svc.AssignServer(teamID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(models.APIResponse{Success: true, Data: ts})
}

// UnassignServer removes a server from a team
func (h *TeamHandler) UnassignServer(c *fiber.Ctx) error {
	teamID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid team ID")
	}
	serverID, err := strconv.ParseUint(c.Params("serverId"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid server ID")
	}

	if err := h.svc.UnassignServer(teamID, serverID); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(models.APIResponse{Success: true, Message: "Server unassigned"})
}

// MigrateFromUserServer migrates old UserServer assignments to team-based assignments
func (h *TeamHandler) MigrateFromUserServer(c *fiber.Ctx) error {
	count, err := h.svc.MigrateFromUserServer()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Migration complete",
		Data:    fiber.Map{"migrated_users": count},
	})
}
