package handlers

import (
	"strconv"

	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber/fiber/v2"
)

type GeoHandler struct {
	svc     *services.GeoService
	userSvc *services.UserService
}

func NewGeoHandler(svc *services.GeoService, userSvc *services.UserService) *GeoHandler {
	return &GeoHandler{svc: svc, userSvc: userSvc}
}

// siteIDWithAccess parses :siteId and checks the caller can access the site's
// server (site routes have no access middleware). 404, not 403, so the response
// does not confirm that the id exists.
func (h *GeoHandler) siteIDWithAccess(c *fiber.Ctx) (uint64, error) {
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

// GetCatalog returns the selectable countries and providers.
func (h *GeoHandler) GetCatalog(c *fiber.Ctx) error {
	return c.JSON(models.APIResponse{Success: true, Data: h.svc.Catalog()})
}

// RefreshProviders re-downloads the provider IP ranges and pushes them to servers in use.
func (h *GeoHandler) RefreshProviders(c *fiber.Ctx) error {
	h.svc.RefreshProviders()
	h.svc.QueueRefreshForActiveServers()
	return c.JSON(models.APIResponse{Success: true, Message: "Provider IP ranges refreshed", Data: h.svc.Catalog()})
}

// AgentProviders serves the cached provider ranges to agents.
func (h *GeoHandler) AgentProviders(c *fiber.Ctx) error {
	return c.JSON(models.APIResponse{Success: true, Data: h.svc.ProvidersForAgent()})
}

func (h *GeoHandler) GetServerRules(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	res, err := h.svc.GetServerRules(serverID)
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to get country access settings"})
	}
	return c.JSON(models.APIResponse{Success: true, Data: res})
}

func (h *GeoHandler) UpdateServerRules(c *fiber.Ctx) error {
	serverID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid server ID"})
	}
	var req models.UpdateGeoRulesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	res, err := h.svc.UpdateServerRules(serverID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Country access updated", Data: res})
}

func (h *GeoHandler) GetSiteRules(c *fiber.Ctx) error {
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

func (h *GeoHandler) UpdateSiteRules(c *fiber.Ctx) error {
	siteID, err := h.siteIDWithAccess(c)
	if siteID == 0 {
		return err
	}
	var req models.UpdateGeoRulesRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	res, err := h.svc.UpdateSiteRules(siteID, req)
	if err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: err.Error()})
	}
	return c.JSON(models.APIResponse{Success: true, Message: "Country access updated", Data: res})
}
