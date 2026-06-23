package handlers

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber-team/wp-manage/internal/api/services"
	"github.com/gofiber-team/wp-manage/internal/models"
)

type FileHandler struct {
	fileSvc *services.FileService
	userSvc *services.UserService
}

func NewFileHandler(fileSvc *services.FileService, userSvc *services.UserService) *FileHandler {
	return &FileHandler{fileSvc: fileSvc, userSvc: userSvc}
}

// getSiteForFileOp retrieves the site, validates it exists, and enforces IDOR protection:
// non-admin users must have explicit access to the site's server before any file operation.
func (h *FileHandler) getSiteForFileOp(c *fiber.Ctx) (*models.Site, error) {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)
	site, err := h.fileSvc.GetSiteWithServer(id)
	if err != nil {
		return nil, c.Status(404).JSON(models.APIResponse{Error: "Site not found"})
	}
	if site.Server == nil {
		return nil, c.Status(400).JSON(models.APIResponse{Error: "Server not found for site"})
	}

	// IDOR guard: verify the authenticated user has access to this site's server.
	// Admins bypass the check; managers/viewers must be assigned to the server.
	role, _ := c.Locals("role").(string)
	if role != "admin" {
		userID, _ := c.Locals("user_id").(uint64)
		if !h.userSvc.HasServerAccess(userID, role, site.ServerID) {
			return nil, c.Status(403).JSON(models.APIResponse{Error: "Access denied: you do not have permission to access files on this site's server"})
		}
	}

	return site, nil
}

// List files in a directory
func (h *FileHandler) List(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	path := c.Query("path", "/")
	params := services.BuildFileQueryParams(site.Domain, path)

	resp, status, err := h.fileSvc.ProxyToAgent(site, "GET", "/files/list", params, nil)
	if err != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + err.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// GetContent reads a file's content
func (h *FileHandler) GetContent(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "path query parameter is required"})
	}

	params := services.BuildFileQueryParams(site.Domain, path)
	resp, status, err := h.fileSvc.ProxyToAgent(site, "GET", "/files/content", params, nil)
	if err != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + err.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// SaveContent writes content to a file
func (h *FileHandler) SaveContent(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, err := h.fileSvc.ProxyToAgent(site, "PUT", "/files/save", params, c.Body())
	if err != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + err.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Upload handles file upload (proxies multipart form to agent)
func (h *FileHandler) Upload(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	destPath := c.Query("path", "/")
	contentType := c.Get("Content-Type")
	body := c.Context().RequestBodyStream()

	resp, status, proxyErr := h.fileSvc.ProxyUpload(site, destPath, body, contentType)
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Download streams a file or directory from agent
func (h *FileHandler) Download(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	path := c.Query("path")
	if path == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "path query parameter is required"})
	}

	resp, proxyErr := h.fileSvc.ProxyDownload(site, path)
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}
	defer resp.Body.Close()

	// Forward headers from agent response
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Set("Content-Type", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		c.Set("Content-Disposition", cd)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		c.Set("Content-Length", cl)
	}

	c.Status(resp.StatusCode)
	// Stream the response body
	_, streamErr := io.Copy(c.Context().Response.BodyWriter(), resp.Body)
	return streamErr
}

// Mkdir creates a new directory
func (h *FileHandler) Mkdir(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/mkdir", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Rename renames a file or directory
func (h *FileHandler) Rename(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "PUT", "/files/rename", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Delete removes a file or directory
func (h *FileHandler) Delete(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/delete", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Permissions changes file permissions
func (h *FileHandler) Permissions(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "PUT", "/files/permissions", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// UploadChunk proxies a single binary chunk to the agent
func (h *FileHandler) UploadChunk(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	uploadID := c.Query("upload_id")
	chunkIndex := c.Query("chunk_index")
	filename := c.Query("filename")
	if uploadID == "" || chunkIndex == "" || filename == "" {
		return c.Status(400).JSON(models.APIResponse{Error: "upload_id, chunk_index, and filename are required"})
	}

	body := c.Context().RequestBodyStream()
	resp, status, proxyErr := h.fileSvc.ProxyUploadChunk(site, uploadID, chunkIndex, filename, body)
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// UploadMerge asks the agent to assemble chunks into the final file
func (h *FileHandler) UploadMerge(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	// Inject domain into body
	var body map[string]interface{}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(400).JSON(models.APIResponse{Error: "Invalid request body"})
	}
	body["domain"] = site.Domain
	// dest_path already set by client, just pass through
	merged, _ := json.Marshal(body)

	resp, status, proxyErr := h.fileSvc.ProxyUploadMerge(site, merged)
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Extract asks the agent to extract an archive file
func (h *FileHandler) Extract(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/extract", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Search searches file contents
func (h *FileHandler) Search(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}

	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/search", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Copy copies one or more files/dirs to a destination directory
func (h *FileHandler) Copy(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}
	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/copy", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Move moves one or more files/dirs to a destination directory
func (h *FileHandler) Move(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}
	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/move", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// DeleteBulk deletes multiple files/dirs at once
func (h *FileHandler) DeleteBulk(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}
	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/delete-bulk", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}

// Compress zips one or more files/dirs into a single archive in a destination directory
func (h *FileHandler) Compress(c *fiber.Ctx) error {
	site, err := h.getSiteForFileOp(c)
	if err != nil {
		return err
	}
	params := services.BuildFileQueryParams(site.Domain, "")
	resp, status, proxyErr := h.fileSvc.ProxyToAgent(site, "POST", "/files/compress", params, c.Body())
	if proxyErr != nil {
		return c.Status(502).JSON(models.APIResponse{Error: "Failed to connect to agent: " + proxyErr.Error()})
	}
	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(resp)
}
