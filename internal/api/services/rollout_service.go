package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type RolloutService struct {
	db *gorm.DB
}

func NewRolloutService(db *gorm.DB) *RolloutService {
	return &RolloutService{db: db}
}

// --- Agent Releases ---

// CreateRelease creates a new agent release
func (s *RolloutService) CreateRelease(req models.CreateReleaseRequest) (*models.AgentRelease, error) {
	// Upsert: if same version exists, update download_url/checksum and return it.
	release := models.AgentRelease{Version: req.Version}
	if err := s.db.Where(models.AgentRelease{Version: req.Version}).
		Assign(models.AgentRelease{
			DownloadURL:  req.DownloadURL,
			Checksum:     req.Checksum,
			ReleaseNotes: req.ReleaseNotes,
			FileSize:     req.FileSize,
		}).
		FirstOrCreate(&release).Error; err != nil {
		return nil, err
	}
	return &release, nil
}

// ListReleases lists all agent releases
func (s *RolloutService) ListReleases() ([]models.AgentRelease, error) {
	var releases []models.AgentRelease
	err := s.db.Order("created_at DESC").Find(&releases).Error
	return releases, err
}

// GetRelease returns a release by ID
func (s *RolloutService) GetRelease(id uint64) (*models.AgentRelease, error) {
	var release models.AgentRelease
	err := s.db.First(&release, id).Error
	return &release, err
}

// DeleteRelease deletes a release
func (s *RolloutService) DeleteRelease(id uint64) error {
	// Check if used in active rollout
	var count int64
	s.db.Model(&models.RolloutPlan{}).Where("release_id = ? AND status IN ?", id, []string{"pending", "running"}).Count(&count)
	if count > 0 {
		return fmt.Errorf("release is in use by an active rollout")
	}
	return s.db.Delete(&models.AgentRelease{}, id).Error
}

// --- Rollout Plans ---

// CreateRollout creates a new rollout plan
// failStaleRollouts marks rollouts that have been pending/running longer than
// maxAge as failed — along with their unfinished servers — so a stuck rollout
// (agent crashed/restarted mid-update and never reported) never permanently
// blocks new rollouts.
func (s *RolloutService) failStaleRollouts(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	var stuck []models.RolloutPlan
	s.db.Where("status IN ? AND created_at < ?", []string{"pending", "running"}, cutoff).Find(&stuck)
	for _, p := range stuck {
		s.db.Model(&models.RolloutServer{}).
			Where("rollout_plan_id = ? AND status IN ?", p.ID, []string{"pending", "updating"}).
			Update("status", "failed")
		now := time.Now()
		s.db.Model(&models.RolloutPlan{}).Where("id = ?", p.ID).
			Updates(map[string]interface{}{"status": "failed", "completed_at": &now})
		slog.Warn("Auto-failed stale rollout", "plan_id", p.ID, "age_minutes", int(time.Since(p.CreatedAt).Minutes()))
	}
}

func (s *RolloutService) CreateRollout(req models.CreateRolloutRequest) (*models.RolloutPlan, error) {
	// Verify release exists
	var release models.AgentRelease
	if err := s.db.First(&release, req.ReleaseID).Error; err != nil {
		return nil, fmt.Errorf("release not found")
	}

	// Clear out rollouts that got stuck: an agent can restart or crash mid-update
	// (or already be on a different version) and never report back, which would
	// otherwise block every future rollout with "another rollout is already in
	// progress". Fail anything older than 15 minutes before checking.
	s.failStaleRollouts(15 * time.Minute)

	// Check no active rollout
	var count int64
	s.db.Model(&models.RolloutPlan{}).Where("status IN ?", []string{"pending", "running"}).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("another rollout is already in progress")
	}

	batchPercent := req.BatchPercent
	if batchPercent == 0 {
		switch req.Strategy {
		case "canary":
			batchPercent = 10
		case "rolling":
			batchPercent = 20
		case "immediate":
			batchPercent = 100
		}
	}

	// Get servers: filtered by server_ids if provided, otherwise all active
	var servers []models.Server
	query := s.db.Where("status = ?", "active")
	if len(req.ServerIDs) > 0 {
		query = query.Where("id IN ?", req.ServerIDs)
	}
	query.Find(&servers)
	if len(servers) == 0 {
		return nil, fmt.Errorf("no active servers found")
	}

	plan := models.RolloutPlan{
		ReleaseID:    req.ReleaseID,
		Strategy:     req.Strategy,
		BatchPercent: batchPercent,
		Status:       "pending",
		TotalServers: len(servers),
	}
	if err := s.db.Create(&plan).Error; err != nil {
		return nil, err
	}

	// Assign servers to batches
	batchSize := int(math.Ceil(float64(len(servers)) * float64(batchPercent) / 100.0))
	if batchSize < 1 {
		batchSize = 1
	}
	for i, server := range servers {
		batchNum := i/batchSize + 1
		rs := models.RolloutServer{
			RolloutPlanID: plan.ID,
			ServerID:      server.ID,
			BatchNumber:   batchNum,
			OldVersion:    server.AgentVersion,
			NewVersion:    release.Version,
			Status:        "pending",
		}
		s.db.Create(&rs)
	}

	plan.Release = &release
	return &plan, nil
}

// StartRollout starts the rollout (queues first batch)
func (s *RolloutService) StartRollout(id uint64) error {
	var plan models.RolloutPlan
	if err := s.db.Preload("Release").First(&plan, id).Error; err != nil {
		return fmt.Errorf("rollout not found")
	}
	if plan.Status != "pending" && plan.Status != "paused" {
		return fmt.Errorf("rollout must be pending or paused to start")
	}

	now := time.Now()
	s.db.Model(&plan).Updates(map[string]interface{}{
		"status":     "running",
		"started_at": &now,
	})

	// Queue first batch
	return s.queueNextBatch(plan.ID)
}

// PauseRollout pauses a running rollout
func (s *RolloutService) PauseRollout(id uint64) error {
	return s.db.Model(&models.RolloutPlan{}).Where("id = ? AND status = ?", id, "running").
		Update("status", "paused").Error
}

// queueNextBatch finds the next batch of servers and queues update tasks
func (s *RolloutService) queueNextBatch(planID uint64) error {
	var plan models.RolloutPlan
	s.db.Preload("Release").First(&plan, planID)

	// Find the first batch with pending servers
	var pendingServers []models.RolloutServer
	s.db.Where("rollout_plan_id = ? AND status = ?", planID, "pending").
		Order("batch_number ASC").
		Find(&pendingServers)

	if len(pendingServers) == 0 {
		// All done
		now := time.Now()
		s.db.Model(&plan).Updates(map[string]interface{}{
			"status":       "completed",
			"completed_at": &now,
		})
		return nil
	}

	// Queue only servers in the first pending batch
	firstBatch := pendingServers[0].BatchNumber
	for _, rs := range pendingServers {
		if rs.BatchNumber != firstBatch {
			break
		}

		now := time.Now()
		s.db.Model(&rs).Updates(map[string]interface{}{
			"status":     "updating",
			"started_at": &now,
		})

		taskData := models.AgentUpdateTask{
			RolloutPlanID: planID,
			ServerID:      rs.ServerID,
			DownloadURL:   plan.Release.DownloadURL,
			Checksum:      plan.Release.Checksum,
			NewVersion:    plan.Release.Version,
		}
		dataJSON, _ := json.Marshal(taskData)
		task := models.TaskQueue{
			ServerID: rs.ServerID,
			TaskType: "agent_update",
			Data:     string(dataJSON),
			Status:   "pending",
		}
		s.db.Create(&task)
	}

	return nil
}

// HandleUpdateResult processes agent update result
func (s *RolloutService) HandleUpdateResult(result models.AgentUpdateResult) error {
	now := time.Now()

	updates := map[string]interface{}{
		"status":       result.Status,
		"completed_at": &now,
	}
	if result.Error != "" {
		updates["error"] = result.Error
	}
	if result.OldVersion != "" {
		updates["old_version"] = result.OldVersion
	}

	s.db.Model(&models.RolloutServer{}).
		Where("rollout_plan_id = ? AND server_id = ?", result.RolloutPlanID, result.ServerID).
		Updates(updates)

	// Update server's agent_version if successful
	if result.Status == "completed" {
		s.db.Model(&models.Server{}).Where("id = ?", result.ServerID).
			Update("agent_version", result.NewVersion)
	}

	// Update plan counters
	var plan models.RolloutPlan
	s.db.First(&plan, result.RolloutPlanID)

	if result.Status == "completed" {
		s.db.Model(&plan).Update("updated_count", gorm.Expr("updated_count + 1"))
	} else if result.Status == "failed" {
		s.db.Model(&plan).Update("failed_count", gorm.Expr("failed_count + 1"))

		// For canary strategy, fail the whole rollout on first failure
		if plan.Strategy == "canary" {
			s.db.Model(&plan).Update("status", "failed")
			return nil
		}
	}

	// Check if current batch is done → queue next
	var updatingCount int64
	s.db.Model(&models.RolloutServer{}).
		Where("rollout_plan_id = ? AND status = ?", result.RolloutPlanID, "updating").
		Count(&updatingCount)

	if updatingCount == 0 && plan.Status == "running" {
		return s.queueNextBatch(result.RolloutPlanID)
	}

	return nil
}

// GetRollout returns a rollout by ID
func (s *RolloutService) GetRollout(id uint64) (*models.RolloutPlan, error) {
	var plan models.RolloutPlan
	err := s.db.Preload("Release").Preload("Servers").Preload("Servers.Server").First(&plan, id).Error
	return &plan, err
}

// ListRollouts lists all rollouts
func (s *RolloutService) ListRollouts(page, perPage int) ([]models.RolloutPlan, int64, error) {
	var plans []models.RolloutPlan
	var total int64

	s.db.Model(&models.RolloutPlan{}).Count(&total)

	err := s.db.Offset((page-1)*perPage).Limit(perPage).
		Preload("Release").
		Order("created_at DESC").
		Find(&plans).Error
	return plans, total, err
}
