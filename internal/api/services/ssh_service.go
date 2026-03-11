package services

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type SSHService struct {
	db *gorm.DB
}

func NewSSHService(db *gorm.DB) *SSHService {
	return &SSHService{db: db}
}

func (s *SSHService) List(serverID uint64) ([]models.SSHKey, error) {
	var keys []models.SSHKey
	err := s.db.Where("server_id = ?", serverID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func (s *SSHService) Create(serverID uint64, req models.CreateSSHKeyRequest) (*models.SSHKey, error) {
	fingerprint := computeFingerprint(req.PublicKey)

	key := models.SSHKey{
		ServerID:    serverID,
		Name:        req.Name,
		PublicKey:   strings.TrimSpace(req.PublicKey),
		Fingerprint: fingerprint,
	}
	if err := s.db.Create(&key).Error; err != nil {
		return nil, err
	}

	// Queue agent task to deploy the key
	taskData, _ := json.Marshal(models.AgentSSHKeyTask{
		Action:    "add",
		Name:      req.Name,
		PublicKey: req.PublicKey,
	})
	s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		TaskType: "add_ssh_key",
		Data:     string(taskData),
		Status:   "pending",
	})

	return &key, nil
}

func (s *SSHService) Delete(serverID, keyID uint64) error {
	var key models.SSHKey
	if err := s.db.Where("id = ? AND server_id = ?", keyID, serverID).First(&key).Error; err != nil {
		return fmt.Errorf("SSH key not found")
	}

	// Queue agent task to remove the key
	taskData, _ := json.Marshal(models.AgentSSHKeyTask{
		Action:    "remove",
		Name:      key.Name,
		PublicKey: key.PublicKey,
	})
	s.db.Create(&models.TaskQueue{
		ServerID: serverID,
		TaskType: "remove_ssh_key",
		Data:     string(taskData),
		Status:   "pending",
	})

	return s.db.Delete(&key).Error
}

func computeFingerprint(pubKey string) string {
	parts := strings.Fields(pubKey)
	if len(parts) < 2 {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	hash := md5.Sum(decoded)
	fp := make([]string, len(hash))
	for i, b := range hash {
		fp[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(fp, ":")
}
