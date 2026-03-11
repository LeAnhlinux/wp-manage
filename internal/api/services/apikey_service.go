package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type APIKeyService struct {
	db *gorm.DB
}

func NewAPIKeyService(db *gorm.DB) *APIKeyService {
	return &APIKeyService{db: db}
}

// Create generates a new API key and returns the plain-text key (only shown once)
func (s *APIKeyService) Create(userID uint64, req models.CreateAPIKeyRequest) (*models.APIKeyModel, string, error) {
	// Generate key: wpm_ + 48 hex chars (24 random bytes)
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate key: %w", err)
	}
	plainKey := "wpm_" + hex.EncodeToString(rawBytes)

	// Hash with SHA256
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	// Prefix for display (wpm_xxxx...)
	keyPrefix := plainKey[:12]

	rateLimit := req.RateLimit
	if rateLimit == 0 {
		rateLimit = 100 // default 100 req/min
	}

	apiKey := models.APIKeyModel{
		UserID:    userID,
		Name:      req.Name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		RateLimit: rateLimit,
	}

	if req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, req.ExpiresIn)
		apiKey.ExpiresAt = &exp
	}

	if err := s.db.Create(&apiKey).Error; err != nil {
		return nil, "", err
	}

	return &apiKey, plainKey, nil
}

// List returns all API keys for a user (without the actual key)
func (s *APIKeyService) List(userID uint64) ([]models.APIKeyModel, error) {
	var keys []models.APIKeyModel
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// Delete removes an API key
func (s *APIKeyService) Delete(id, userID uint64) error {
	result := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.APIKeyModel{})
	if result.RowsAffected == 0 {
		return errors.New("API key not found")
	}
	return result.Error
}

// ValidateKey checks a plain-text API key and returns the associated user
func (s *APIKeyService) ValidateKey(plainKey string) (*models.APIKeyModel, error) {
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	var apiKey models.APIKeyModel
	if err := s.db.Where("key_hash = ?", keyHash).Preload("User").First(&apiKey).Error; err != nil {
		return nil, errors.New("invalid API key")
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, errors.New("API key expired")
	}

	// Update last_used_at
	now := time.Now()
	s.db.Model(&apiKey).Update("last_used_at", &now)

	return &apiKey, nil
}
