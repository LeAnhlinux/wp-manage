package services

import (
	"errors"
	"strings"

	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) Create(req models.CreateUserRequest) (*models.User, error) {
	// M5: Validate email format
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRegex.MatchString(req.Email) {
		return nil, errors.New("invalid email address format")
	}

	// M2: Enforce password strength
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	var existing models.User
	if err := s.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return nil, errors.New("email already registered")
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Name:         req.Name,
		Role:         req.Role,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) List(page, perPage int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	s.db.Model(&models.User{}).Count(&total)
	err := s.db.Offset((page - 1) * perPage).Limit(perPage).
		Order("created_at DESC").
		Find(&users).Error
	return users, total, err
}

func (s *UserService) GetByID(id uint64) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) Update(id uint64, req models.UpdateUserRequest) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, errors.New("user not found")
	}

	updates := map[string]interface{}{}
	if req.Email != nil {
		// M5: Validate email format
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if !emailRegex.MatchString(email) {
			return nil, errors.New("invalid email address format")
		}
		// Check uniqueness
		var existing models.User
		if err := s.db.Where("email = ? AND id != ?", email, id).First(&existing).Error; err == nil {
			return nil, errors.New("email already in use")
		}
		updates["email"] = email
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.Password != nil {
		// M2: Enforce password strength on update
		if err := validatePassword(*req.Password); err != nil {
			return nil, err
		}
		hash, err := utils.HashPassword(*req.Password)
		if err != nil {
			return nil, err
		}
		updates["password_hash"] = hash
	}

	if len(updates) > 0 {
		if err := s.db.Model(&user).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	// Reload
	s.db.First(&user, id)
	return &user, nil
}

func (s *UserService) Delete(id uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove server assignments
		tx.Where("user_id = ?", id).Delete(&models.UserServer{})
		// Remove API keys
		tx.Where("user_id = ?", id).Delete(&models.APIKeyModel{})
		// Delete user
		return tx.Delete(&models.User{}, id).Error
	})
}

// AssignServer assigns a server to a user
func (s *UserService) AssignServer(userID, serverID uint64) error {
	// Check user exists
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return errors.New("user not found")
	}
	// Check server exists
	var server models.Server
	if err := s.db.First(&server, serverID).Error; err != nil {
		return errors.New("server not found")
	}
	// Check if already assigned
	var existing models.UserServer
	if err := s.db.Where("user_id = ? AND server_id = ?", userID, serverID).First(&existing).Error; err == nil {
		return errors.New("server already assigned to this user")
	}

	us := models.UserServer{
		UserID:   userID,
		ServerID: serverID,
	}
	return s.db.Create(&us).Error
}

// UnassignServer removes a server assignment from a user
func (s *UserService) UnassignServer(userID, serverID uint64) error {
	result := s.db.Where("user_id = ? AND server_id = ?", userID, serverID).Delete(&models.UserServer{})
	if result.RowsAffected == 0 {
		return errors.New("assignment not found")
	}
	return result.Error
}

// GetAssignedServers returns servers assigned to a user
func (s *UserService) GetAssignedServers(userID uint64) ([]models.Server, error) {
	var servers []models.Server
	err := s.db.Joins("JOIN user_servers ON user_servers.server_id = servers.id").
		Where("user_servers.user_id = ?", userID).
		Preload("Sites").
		Find(&servers).Error
	return servers, err
}

// GetAccessibleServerIDs returns server IDs accessible by a user based on role
// Uses both TeamServer+TeamMember (new) and UserServer (legacy) chains
func (s *UserService) GetAccessibleServerIDs(userID uint64, role string) ([]uint64, error) {
	if role == "admin" {
		var ids []uint64
		err := s.db.Model(&models.Server{}).Pluck("id", &ids).Error
		return ids, err
	}

	// Combine: servers from team membership + direct UserServer assignments
	var ids []uint64
	err := s.db.Raw(`
		SELECT DISTINCT server_id FROM (
			SELECT server_id FROM team_servers WHERE team_id IN (
				SELECT team_id FROM team_members WHERE user_id = ?
			)
			UNION
			SELECT server_id FROM user_servers WHERE user_id = ?
		) AS combined
	`, userID, userID).Pluck("server_id", &ids).Error
	return ids, err
}

// HasServerAccess checks if a user can access a specific server
// Uses both TeamServer+TeamMember (new) and UserServer (legacy) chains
func (s *UserService) HasServerAccess(userID uint64, role string, serverID uint64) bool {
	if role == "admin" {
		return true
	}

	// Check team-based access
	var teamCount int64
	s.db.Model(&models.TeamServer{}).Where("server_id = ? AND team_id IN (?)",
		serverID,
		s.db.Model(&models.TeamMember{}).Select("team_id").Where("user_id = ?", userID),
	).Count(&teamCount)
	if teamCount > 0 {
		return true
	}

	// Fallback: check legacy UserServer
	var directCount int64
	s.db.Model(&models.UserServer{}).Where("user_id = ? AND server_id = ?", userID, serverID).Count(&directCount)
	return directCount > 0
}
