package handlers

import (
	"strconv"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber/fiber/v2"
)

type BotHandler struct {
	svc     *services.BotService
	userSvc *services.UserService
}

func NewBotHandler(svc *services.BotService, userSvc *services.UserService) *BotHandler {
	return &BotHandler{svc: svc, userSvc: userSvc}
}

// siteIDWithAccess parses :siteId and checks the caller can access the site's
// server. Site routes have no access middleware, so the handler must check.
// A site the caller may not see is reported as 404, not 403, so the response
// does not confirm that the id exists.
func (h *BotHandler) siteIDWithAccess(c *fiber.Ctx) (uint64, error) {
	siteID, err := strconv.ParseUint(c.Params("siteId"), 10, 64)
	if err != nil || siteID == 0 {
		return 0, c.Status(400).JSON(models.APIResponse{Error: "Invalid site ID"})
	}
	serverID, ok := h.svc.SiteServerID(siteID)
	if ok {
		role, _ := c.Locals("role").(string)
		userID, _ := c.Locals("user_id").(uint64)
		ok = h.userSvc.HasServerAccess(userID, role, serverID)
	}
	if !ok {
		return 0, c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	return siteID, nil
}

// GetCatalog returns the preset bots and the allowed rate tiers.
func (h *BotHandler) GetCatalog(c *fiber.Ctx) error {
	return c.JSON(models.APIResponse{Success: true, Data: fiber.Map{
		"bots":       models.BotCatalog,
		"rate_tiers": models.BotRateTiers,
	}})
}

func (h *BotHandler) GetServerRules(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	res, err := h.svc.GetServerRules(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get bot rules"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: res})
}

func (h *BotHandler) UpdateServerRules(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	var req models.UpdateBotRulesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	res, err := h.svc.UpdateServerRules(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Bot rules updated", Data: res})
}

func (h *BotHandler) GetSiteRules(c *fiber.Ctx) error {
	siteID, err := h.siteIDWithAccess(c)
	if siteID == 0 {
		return err
	}
	res, err := h.svc.GetSiteRules(siteID)
	if err != nil {
		return c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: res})
}

func (h *BotHandler) UpdateSiteRules(c *fiber.Ctx) error {
	siteID, err := h.siteIDWithAccess(c)
	if siteID == 0 {
		return err
	}
	var req models.UpdateBotRulesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	res, err := h.svc.UpdateSiteRules(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Bot rules updated", Data: res})
}
