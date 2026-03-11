package services

import (
	"errors"
	"fmt"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm"
)

type TeamService struct {
	db *gorm.DB
}

func NewTeamService(db *gorm.DB) *TeamService {
	return &TeamService{db: db}
}

// Create creates a new team with the creator as owner and admin member
func (s *TeamService) Create(ownerID uint64, req models.CreateTeamRequest) (*models.Team, error) {
	team := models.Team{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     ownerID,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		// Add owner as admin member automatically
		member := models.TeamMember{
			TeamID: team.ID,
			UserID: ownerID,
			Role:   "admin",
		}
		return tx.Create(&member).Error
	})
	if err != nil {
		return nil, err
	}

	// Reload with associations
	s.db.Preload("Owner").Preload("Members.User").Preload("Servers.Server").First(&team, team.ID)
	return &team, nil
}

// List returns teams visible to the user
func (s *TeamService) List(userID uint64, role string, page, perPage int) ([]models.Team, int64, error) {
	var teams []models.Team
	var total int64

	query := s.db.Model(&models.Team{})
	if role != "admin" {
		// Non-admin: only show teams they're a member of
		query = query.Where("id IN (?)",
			s.db.Model(&models.TeamMember{}).Select("team_id").Where("user_id = ?", userID),
		)
	}
	query.Count(&total)

	err := query.Offset((page-1)*perPage).Limit(perPage).
		Preload("Owner").
		Preload("Members.User").
		Preload("Servers.Server").
		Order("created_at DESC").
		Find(&teams).Error

	return teams, total, err
}

// GetByID returns a team by ID
func (s *TeamService) GetByID(teamID uint64) (*models.Team, error) {
	var team models.Team
	err := s.db.Preload("Owner").
		Preload("Members.User").
		Preload("Servers.Server").
		First(&team, teamID).Error
	if err != nil {
		return nil, errors.New("team not found")
	}
	return &team, nil
}

// Update updates team details
func (s *TeamService) Update(teamID uint64, req models.UpdateTeamRequest) (*models.Team, error) {
	var team models.Team
	if err := s.db.First(&team, teamID).Error; err != nil {
		return nil, errors.New("team not found")
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) > 0 {
		if err := s.db.Model(&team).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return s.GetByID(teamID)
}

// Delete removes a team and all its associations
func (s *TeamService) Delete(teamID uint64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("team_id = ?", teamID).Delete(&models.TeamServer{})
		tx.Where("team_id = ?", teamID).Delete(&models.TeamMember{})
		return tx.Delete(&models.Team{}, teamID).Error
	})
}

// HasTeamAccess checks if user is a member of the team (or admin role)
func (s *TeamService) HasTeamAccess(userID uint64, role string, teamID uint64) bool {
	if role == "admin" {
		return true
	}
	var count int64
	s.db.Model(&models.TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Count(&count)
	return count > 0
}

// IsTeamAdmin checks if user is an admin of the team (or system admin)
func (s *TeamService) IsTeamAdmin(userID uint64, role string, teamID uint64) bool {
	if role == "admin" {
		return true
	}
	var count int64
	s.db.Model(&models.TeamMember{}).Where("team_id = ? AND user_id = ? AND role = 'admin'", teamID, userID).Count(&count)
	return count > 0
}

// --- Members ---

// AddMember adds a user to a team
func (s *TeamService) AddMember(teamID uint64, req models.AddTeamMemberRequest) (*models.TeamMember, error) {
	// Check team exists
	var team models.Team
	if err := s.db.First(&team, teamID).Error; err != nil {
		return nil, errors.New("team not found")
	}
	// Check user exists
	var user models.User
	if err := s.db.First(&user, req.UserID).Error; err != nil {
		return nil, errors.New("user not found")
	}
	// Check not already member
	var existing models.TeamMember
	if err := s.db.Where("team_id = ? AND user_id = ?", teamID, req.UserID).First(&existing).Error; err == nil {
		return nil, errors.New("user is already a member of this team")
	}

	member := models.TeamMember{
		TeamID: teamID,
		UserID: req.UserID,
		Role:   req.Role,
	}
	if err := s.db.Create(&member).Error; err != nil {
		return nil, err
	}

	s.db.Preload("User").First(&member, member.ID)
	return &member, nil
}

// UpdateMember updates a member's role in a team
func (s *TeamService) UpdateMember(teamID, userID uint64, req models.UpdateTeamMemberRequest) (*models.TeamMember, error) {
	var member models.TeamMember
	if err := s.db.Where("team_id = ? AND user_id = ?", teamID, userID).First(&member).Error; err != nil {
		return nil, errors.New("member not found")
	}

	member.Role = req.Role
	if err := s.db.Save(&member).Error; err != nil {
		return nil, err
	}

	s.db.Preload("User").First(&member, member.ID)
	return &member, nil
}

// RemoveMember removes a user from a team
func (s *TeamService) RemoveMember(teamID, userID uint64) error {
	// Cannot remove the owner
	var team models.Team
	if err := s.db.First(&team, teamID).Error; err != nil {
		return errors.New("team not found")
	}
	if team.OwnerID == userID {
		return errors.New("cannot remove team owner")
	}

	result := s.db.Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&models.TeamMember{})
	if result.RowsAffected == 0 {
		return errors.New("member not found")
	}
	return result.Error
}

// --- Servers ---

// AssignServer adds a server to a team
func (s *TeamService) AssignServer(teamID uint64, req models.AssignTeamServerRequest) (*models.TeamServer, error) {
	// Check team exists
	var team models.Team
	if err := s.db.First(&team, teamID).Error; err != nil {
		return nil, errors.New("team not found")
	}
	// Check server exists
	var server models.Server
	if err := s.db.First(&server, req.ServerID).Error; err != nil {
		return nil, errors.New("server not found")
	}
	// Check not already assigned
	var existing models.TeamServer
	if err := s.db.Where("team_id = ? AND server_id = ?", teamID, req.ServerID).First(&existing).Error; err == nil {
		return nil, errors.New("server already assigned to this team")
	}

	ts := models.TeamServer{
		TeamID:   teamID,
		ServerID: req.ServerID,
	}
	if err := s.db.Create(&ts).Error; err != nil {
		return nil, err
	}

	s.db.Preload("Server").First(&ts, ts.ID)
	return &ts, nil
}

// UnassignServer removes a server from a team
func (s *TeamService) UnassignServer(teamID, serverID uint64) error {
	result := s.db.Where("team_id = ? AND server_id = ?", teamID, serverID).Delete(&models.TeamServer{})
	if result.RowsAffected == 0 {
		return errors.New("server assignment not found")
	}
	return result.Error
}

// --- Migration helper ---

// MigrateFromUserServer migrates existing UserServer assignments to teams
// For each user with UserServer entries, creates a personal team and copies assignments
func (s *TeamService) MigrateFromUserServer() (int, error) {
	// Get all unique user IDs that have UserServer entries
	var userIDs []uint64
	if err := s.db.Model(&models.UserServer{}).Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
		return 0, err
	}

	migrated := 0
	for _, userID := range userIDs {
		var user models.User
		if err := s.db.First(&user, userID).Error; err != nil {
			continue
		}

		// Check if user already has a team
		var existingCount int64
		s.db.Model(&models.TeamMember{}).Where("user_id = ?", userID).Count(&existingCount)
		if existingCount > 0 {
			continue
		}

		// Create personal team
		team := models.Team{
			Name:        fmt.Sprintf("%s's Team", user.Name),
			Description: "Auto-created from server assignments",
			OwnerID:     userID,
		}

		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&team).Error; err != nil {
				return err
			}
			// Add user as admin member
			member := models.TeamMember{TeamID: team.ID, UserID: userID, Role: "admin"}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
			// Copy UserServer assignments to TeamServer
			var userServers []models.UserServer
			tx.Where("user_id = ?", userID).Find(&userServers)
			for _, us := range userServers {
				ts := models.TeamServer{TeamID: team.ID, ServerID: us.ServerID}
				tx.Create(&ts)
			}
			return nil
		})

		if err == nil {
			migrated++
		}
	}

	return migrated, nil
}
