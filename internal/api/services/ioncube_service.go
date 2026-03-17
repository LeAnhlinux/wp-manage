package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

// IonCubeVersionStatus mirrors agent/ioncube.VersionStatus.
type IonCubeVersionStatus struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
}

type IonCubeService struct {
	db *gorm.DB
}

func NewIonCubeService(db *gorm.DB) *IonCubeService {
	return &IonCubeService{db: db}
}

// GetStatus asks the agent to report ionCube status for all installed PHP versions.
func (s *IonCubeService) GetStatus(serverID uint64) ([]IonCubeVersionStatus, error) {
	taskData := models.AgentIonCubeTask{Action: "get_status"}
	task := s.queueTask(serverID, taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	var statuses []IonCubeVersionStatus
	if result.Output != "" {
		if err := json.Unmarshal([]byte(result.Output), &statuses); err != nil {
			return nil, fmt.Errorf("failed to parse ionCube status: %w", err)
		}
	}
	return statuses, nil
}

// Enable downloads (if needed), installs, and activates ionCube for phpVersion.
// Uses a 5-minute timeout because first enable may require downloading the ~60 MB tarball.
func (s *IonCubeService) Enable(serverID uint64, phpVersion string) error {
	taskData := models.AgentIonCubeTask{Action: "enable", PHPVersion: phpVersion}
	task := s.queueTask(serverID, taskData)

	result, err := WaitForTaskCompleteWithTimeout(s.db, task.ID, 5*time.Minute)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

// Disable removes ionCube ini files and reloads PHP-FPM for phpVersion.
func (s *IonCubeService) Disable(serverID uint64, phpVersion string) error {
	taskData := models.AgentIonCubeTask{Action: "disable", PHPVersion: phpVersion}
	task := s.queueTask(serverID, taskData)

	result, err := WaitForTaskComplete(s.db, task.ID)
	if err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func (s *IonCubeService) queueTask(serverID uint64, data interface{}) *models.TaskQueue {
	dataJSON, _ := json.Marshal(data)
	task := models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_ioncube",
		Data:     string(dataJSON),
		Status:   "pending",
	}
	s.db.Create(&task)
	return &task
}
