package services

import (
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type PanelUpdateService struct {
	db             *gorm.DB
	currentVersion string
}

func NewPanelUpdateService(db *gorm.DB, currentVersion string) *PanelUpdateService {
	return &PanelUpdateService{db: db, currentVersion: currentVersion}
}

func (s *PanelUpdateService) GetCurrentVersion() string {
	return s.currentVersion
}

func (s *PanelUpdateService) GetLatestRelease() (*models.PanelRelease, error) {
	var release models.PanelRelease
	err := s.db.Order("created_at DESC").First(&release).Error
	if err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *PanelUpdateService) CreateRelease(req models.CreatePanelReleaseRequest) (*models.PanelRelease, error) {
	release := models.PanelRelease{
		Version:      req.Version,
		DownloadURL:  req.DownloadURL,
		Checksum:     req.Checksum,
		ReleaseNotes: req.ReleaseNotes,
		FileSize:     req.FileSize,
	}
	if err := s.db.Create(&release).Error; err != nil {
		return nil, err
	}
	return &release, nil
}

func (s *PanelUpdateService) ListReleases() ([]models.PanelRelease, error) {
	var releases []models.PanelRelease
	err := s.db.Order("created_at DESC").Limit(20).Find(&releases).Error
	return releases, err
}
