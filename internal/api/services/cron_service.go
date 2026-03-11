package services

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type CronService struct {
	db *gorm.DB
}

func NewCronService(db *gorm.DB) *CronService {
	return &CronService{db: db}
}

func (s *CronService) ListBySite(siteID uint64) ([]models.CronJob, error) {
	var jobs []models.CronJob
	err := s.db.Where("site_id = ?", siteID).Order("created_at DESC").Find(&jobs).Error
	return jobs, err
}

func (s *CronService) ListByServer(serverID uint64) ([]models.CronJob, error) {
	var jobs []models.CronJob
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&jobs).Error
	return jobs, err
}

func (s *CronService) Create(serverID uint64, req models.CreateCronJobRequest) (*models.CronJob, error) {
	if req.Command == "" || req.Schedule == "" {
		return nil, fmt.Errorf("command and schedule are required")
	}

	user := req.User
	if user == "" {
		user = "root"
	}

	job := models.CronJob{
		ServerID: serverID,
		SiteID:   req.SiteID,
		Command:  req.Command,
		Schedule: req.Schedule,
		User:     user,
		Enabled:  true,
		Comment:  req.Comment,
	}

	if err := s.db.Create(&job).Error; err != nil {
		return nil, err
	}

	// Queue agent task to sync cron (fire-and-forget)
	s.queueCronSync(serverID, "add", job)
	return &job, nil
}

func (s *CronService) Update(serverID, jobID uint64, req models.UpdateCronJobRequest) (*models.CronJob, error) {
	var job models.CronJob
	if err := s.db.Where("id = ? AND server_id = ?", jobID, serverID).First(&job).Error; err != nil {
		return nil, fmt.Errorf("cron job not found")
	}

	if req.Command != nil {
		job.Command = *req.Command
	}
	if req.Schedule != nil {
		job.Schedule = *req.Schedule
	}
	if req.User != nil {
		job.User = *req.User
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	if req.Comment != nil {
		job.Comment = *req.Comment
	}

	if err := s.db.Save(&job).Error; err != nil {
		return nil, err
	}

	s.queueCronSync(serverID, "add", job)
	return &job, nil
}

func (s *CronService) Toggle(serverID, jobID uint64) (*models.CronJob, error) {
	var job models.CronJob
	if err := s.db.Where("id = ? AND server_id = ?", jobID, serverID).First(&job).Error; err != nil {
		return nil, fmt.Errorf("cron job not found")
	}

	job.Enabled = !job.Enabled
	if err := s.db.Save(&job).Error; err != nil {
		return nil, err
	}

	action := "add"
	if !job.Enabled {
		action = "remove"
	}
	s.queueCronSync(serverID, action, job)
	return &job, nil
}

func (s *CronService) Delete(serverID, jobID uint64) error {
	var job models.CronJob
	if err := s.db.Where("id = ? AND server_id = ?", jobID, serverID).First(&job).Error; err != nil {
		return fmt.Errorf("cron job not found")
	}

	// Queue removal on agent first
	s.queueCronSync(serverID, "remove", job)

	return s.db.Delete(&job).Error
}

// RunNow executes a cron job immediately and returns its output (synchronous)
func (s *CronService) RunNow(serverID, jobID uint64) (string, error) {
	var job models.CronJob
	if err := s.db.Where("id = ? AND server_id = ?", jobID, serverID).First(&job).Error; err != nil {
		return "", fmt.Errorf("cron job not found")
	}

	task := models.AgentCronTask{
		Action:  "run_now",
		JobID:   job.ID,
		User:    job.User,
		Command: job.Command,
	}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_cron",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return "", err
	}

	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return "", err
	}
	if result.Error != "" {
		return result.Output, fmt.Errorf("%s", result.Error)
	}
	return result.Output, nil
}

// GetLogs retrieves recent log output for a cron job (synchronous)
func (s *CronService) GetLogs(serverID, jobID uint64, lines int) (string, error) {
	var job models.CronJob
	if err := s.db.Where("id = ? AND server_id = ?", jobID, serverID).First(&job).Error; err != nil {
		return "", fmt.Errorf("cron job not found")
	}

	if lines <= 0 {
		lines = 100
	}

	task := models.AgentCronTask{
		Action: "get_logs",
		JobID:  job.ID,
		Lines:  lines,
	}
	data, _ := json.Marshal(task)

	queuedTask := models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_cron",
		Data:     string(data),
		Status:   "pending",
	}
	if err := s.db.Create(&queuedTask).Error; err != nil {
		return "", err
	}

	result, err := WaitForTaskComplete(s.db, queuedTask.ID)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (s *CronService) queueCronSync(serverID uint64, action string, job models.CronJob) {
	task := models.AgentCronTask{
		Action:   action,
		JobID:    job.ID,
		User:     job.User,
		Schedule: job.Schedule,
		Command:  job.Command,
		Comment:  job.Comment,
		Enabled:  job.Enabled,
	}
	data, _ := json.Marshal(task)
	s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		TaskType: "manage_cron",
		Data:     string(data),
		Status:   "pending",
	})
}
