package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type HardeningService struct {
	db *gorm.DB
}

func NewHardeningService(db *gorm.DB) *HardeningService {
	return &HardeningService{db: db}
}

func (s *HardeningService) Harden(siteID uint64, req models.HardenWordPressRequest) error {
	var site models.Site
	if err := s.db.First(&site, siteID).Error; err != nil {
		return fmt.Errorf("site not found")
	}

	taskData, _ := json.Marshal(models.AgentHardenTask{
		Domain:            site.Domain,
		DisableXMLRPC:     req.DisableXMLRPC,
		HideWPVersion:     req.HideWPVersion,
		SecureFilePerms:   req.SecureFilePerms,
		SecurityHeaders:   req.SecurityHeaders,
		DisableFileEditor: req.DisableFileEditor,
	})

	return s.db.Create(&models.TaskQueue{
		ServerID: site.ServerID,
		SiteID:   siteID,
		TaskType: "harden_wordpress",
		Data:     string(taskData),
		Status:   "pending",
	}).Error
}
