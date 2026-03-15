package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type HardeningService struct {
	db *gorm.DB
}

func NewHardeningService(db *gorm.DB) *HardeningService {
	return &HardeningService{db: db}
}

// Harden applies WordPress hardening measures and waits for agent confirmation (synchronous).
func (s *HardeningService) Harden(siteID uint64, req models.HardenWordPressRequest) error {
	return s.hardenOrRevert(siteID, req, false)
}

// RevertHarden removes previously applied WordPress hardening measures (synchronous).
func (s *HardeningService) RevertHarden(siteID uint64, req models.HardenWordPressRequest) error {
	return s.hardenOrRevert(siteID, req, true)
}

func (s *HardeningService) hardenOrRevert(siteID uint64, req models.HardenWordPressRequest, revert bool) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	taskData, _ := json.Marshal(models.AgentHardenTask{
		Domain:            site.Domain,
		Revert:            revert,
		DisableXMLRPC:     req.DisableXMLRPC,
		HideWPVersion:     req.HideWPVersion,
		SecureFilePerms:   req.SecureFilePerms,
		SecurityHeaders:   req.SecurityHeaders,
		DisableFileEditor: req.DisableFileEditor,
	})

	task := models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "harden_wordpress",
		Data:     string(taskData),
		Status:   "pending",
	}
	if err := s.db.Create(&task).Error; err != nil {
		return fmt.Errorf("failed to queue task: %w", err)
	}

	// Wait for agent to complete (max 60s)
	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 60*time.Second)
	if err != nil {
		return fmt.Errorf("agent timeout: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}
