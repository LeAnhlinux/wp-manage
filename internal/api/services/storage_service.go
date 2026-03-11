package services

import (
	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type StorageService struct {
	db *gorm.DB
}

func NewStorageService(db *gorm.DB) *StorageService {
	return &StorageService{db: db}
}

func (s *StorageService) Create(serverID uint64, req models.CreateStorageCredentialRequest) (*models.StorageCredential, error) {
	cred := models.StorageCredential{
		ServerID:  serverID,
		Provider:  req.Provider,
		Bucket:    req.Bucket,
		Region:    req.Region,
		AccessKey: req.AccessKey,
		SecretKey: req.SecretKey,
		Endpoint:  req.Endpoint,
		BasePath:  req.BasePath,
	}
	if err := s.db.Create(&cred).Error; err != nil {
		return nil, err
	}
	return &cred, nil
}

func (s *StorageService) List(serverID uint64) ([]models.StorageCredential, error) {
	var creds []models.StorageCredential
	err := s.db.Where("server_id = ?", serverID).Find(&creds).Error
	return creds, err
}

func (s *StorageService) Delete(id uint64) error {
	return s.db.Delete(&models.StorageCredential{}, id).Error
}

func (s *StorageService) GetByProvider(serverID uint64, provider string) (*models.StorageCredential, error) {
	var cred models.StorageCredential
	err := s.db.Where("server_id = ? AND provider = ?", serverID, provider).First(&cred).Error
	if err != nil {
		return nil, err
	}
	return &cred, nil
}
