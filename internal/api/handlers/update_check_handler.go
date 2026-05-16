package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

const mirrorBaseURL = "https://mirrors.bucloud.com"

// gitSuffixRe matches git-describe suffixes appended to version tags:
//
//	"-dirty"                → local uncommitted changes on exact tag
//	"-3-gabcdef1"           → 3 commits after tag, hash gabcdef1
//	"-3-gabcdef1-dirty"     → commits after tag + uncommitted changes
//
// Stripping these gives the base semver tag so we can compare with the clean
// mirror version without false "needs update" alerts after a manual deploy.
var gitSuffixRe = regexp.MustCompile(`-\d+-g[0-9a-f]+(-dirty)?$|-dirty$`)

// normalizeVersion strips git-describe suffixes from a version string.
// "v1.3.15-dirty" → "v1.3.15", "v1.3.12-3-gabcdef1-dirty" → "v1.3.12", "v1.3.15" → "v1.3.15".
func normalizeVersion(v string) string {
	return gitSuffixRe.ReplaceAllString(v, "")
}

type UpdateCheckHandler struct {
	panelSvc   *services.PanelUpdateService
	rolloutSvc *services.RolloutService
	serverSvc  *services.ServerService
}

func NewUpdateCheckHandler(panelSvc *services.PanelUpdateService, rolloutSvc *services.RolloutService, serverSvc *services.ServerService) *UpdateCheckHandler {
	return &UpdateCheckHandler{panelSvc: panelSvc, rolloutSvc: rolloutSvc, serverSvc: serverSvc}
}

type VersionManifest struct {
	Version      string     `json:"version"`
	ReleasedAt   string     `json:"released_at"`
	ReleaseNotes string     `json:"release_notes"`
	Panel        BinaryInfo `json:"panel"`
	Agent        BinaryInfo `json:"agent"`
	Web          BinaryInfo `json:"web"`
}

type BinaryInfo struct {
	DownloadURL string `json:"download_url"`
	Checksum    string `json:"checksum"`
}

type ServerAgentInfo struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	IPAddress    string `json:"ip_address"`
	AgentVersion string `json:"agent_version"`
	NeedsUpdate  bool   `json:"needs_update"`
}

type CheckUpdateResult struct {
	CurrentPanel  string            `json:"current_panel"`
	CurrentAgent  string            `json:"current_agent"`
	Latest        *VersionManifest  `json:"latest"`
	PanelNewer    bool              `json:"panel_newer"`
	AgentNewer    bool              `json:"agent_newer"`
	ServerAgents  []ServerAgentInfo `json:"server_agents"`
}

// CheckUpdate fetches version.json from mirror and compares with running versions.
// GET /settings/update/check
func (h *UpdateCheckHandler) CheckUpdate(c *fiber.Ctx) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(mirrorBaseURL + "/version.json")
	if err != nil {
		return c.Status(502).JSON(models.APIResponse{Error: fmt.Sprintf("Cannot reach mirror: %v", err)})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to read version.json from mirror"})
	}

	var manifest VersionManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Invalid version.json format"})
	}

	currentPanel := h.panelSvc.GetCurrentVersion()
	// normalizeVersion strips git-describe suffixes ("-dirty", "-3-gabcdef1-dirty", …)
	// so a locally-deployed binary built from the same codebase as the latest mirror
	// release does NOT show a false "needs update" in the UI.
	panelNewer := manifest.Version != "" && manifest.Version != normalizeVersion(currentPanel)

	// Build per-server agent info
	servers, _, _ := h.serverSvc.List(0, "admin", 1, 200)
	serverAgents := make([]ServerAgentInfo, 0, len(servers))
	agentNewer := false
	for _, s := range servers {
		needsUpdate := manifest.Version != "" && normalizeVersion(s.AgentVersion) != manifest.Version
		if needsUpdate {
			agentNewer = true
		}
		serverAgents = append(serverAgents, ServerAgentInfo{
			ID:           s.ID,
			Name:         s.Name,
			IPAddress:    s.IPAddress,
			AgentVersion: s.AgentVersion,
			NeedsUpdate:  needsUpdate,
		})
	}

	// current_agent: most common version (for display summary)
	agentVersions := map[string]int{}
	for _, s := range servers {
		if s.AgentVersion != "" {
			agentVersions[s.AgentVersion]++
		}
	}
	currentAgent := ""
	maxCount := 0
	for v, count := range agentVersions {
		if count > maxCount {
			currentAgent = v
			maxCount = count
		}
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Data: CheckUpdateResult{
			CurrentPanel: currentPanel,
			CurrentAgent: currentAgent,
			Latest:       &manifest,
			PanelNewer:   panelNewer,
			AgentNewer:   agentNewer,
			ServerAgents: serverAgents,
		},
	})
}

// ApplyPanelUpdate creates a release record and triggers panel binary + web archive swap.
// POST /settings/update/apply-panel
// Body: {version, download_url, checksum, web_url, web_checksum}
func (h *UpdateCheckHandler) ApplyPanelUpdate(c *fiber.Ctx) error {
	var req struct {
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		Checksum    string `json:"checksum"`
		WebURL      string `json:"web_url"`
		WebChecksum string `json:"web_checksum"`
	}
	if err := c.BodyParser(&req); err != nil || req.Version == "" || req.DownloadURL == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "version and download_url are required"})
	}

	release, err := h.panelSvc.CreateRelease(models.CreatePanelReleaseRequest{
		Version:      req.Version,
		DownloadURL:  req.DownloadURL,
		Checksum:     req.Checksum,
		WebURL:       req.WebURL,
		WebChecksum:  req.WebChecksum,
		ReleaseNotes: "Applied via panel auto-update",
	})
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to create release: " + err.Error()})
	}

	// Return 202 BEFORE triggering the update so the HTTP response reaches the
	// client before the updater stops this process. A 1-second delay gives
	// Fiber time to flush the response and Nginx to forward it.
	go func(releaseID uint64) {
		time.Sleep(1 * time.Second)
		if err := h.panelSvc.TriggerUpdate(releaseID); err != nil {
			slog.Error("panel self-update failed", "err", err)
		}
	}(release.ID)
	return c.Status(202).JSON(models.APIResponse{Success: true, Message: "Panel update triggered — restarting shortly"})
}

// ApplyAgentUpdate creates a release + immediate rollout to update all agents.
// POST /settings/update/apply-agents
// Body: {version, download_url, checksum}
func (h *UpdateCheckHandler) ApplyAgentUpdate(c *fiber.Ctx) error {
	var req struct {
		Version     string   `json:"version"`
		DownloadURL string   `json:"download_url"`
		Checksum    string   `json:"checksum"`
		ServerIDs   []uint64 `json:"server_ids"` // optional — empty = all active servers
	}
	if err := c.BodyParser(&req); err != nil || req.Version == "" || req.DownloadURL == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "version and download_url are required"})
	}

	release, err := h.rolloutSvc.CreateRelease(models.CreateReleaseRequest{
		Version:      req.Version,
		DownloadURL:  req.DownloadURL,
		Checksum:     req.Checksum,
		ReleaseNotes: "Applied via panel auto-update",
		FileSize:     0,
	})
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to create release: " + err.Error()})
	}

	rollout, err := h.rolloutSvc.CreateRollout(models.CreateRolloutRequest{
		ReleaseID:    release.ID,
		Strategy:     "immediate",
		BatchPercent: 100,
		ServerIDs:    req.ServerIDs,
	})
	if err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to create rollout: " + err.Error()})
	}

	if err := h.rolloutSvc.StartRollout(rollout.ID); err != nil {
		return c.Status(500).JSON(models.APIResponse{Error: "Failed to start rollout: " + err.Error()})
	}

	return c.JSON(models.APIResponse{
		Success: true,
		Message: "Agent update started",
		Data:    fiber.Map{"rollout_id": rollout.ID},
	})
}
