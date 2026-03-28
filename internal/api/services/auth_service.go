package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/pquerna/otp/totp"

	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

// emailRegex validates basic email format (RFC 5322 simplified).
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// validatePassword enforces minimum password strength:
// - At least 8 characters
// - At least 1 uppercase letter
// - At least 1 lowercase letter
// - At least 1 digit
func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	return nil
}

type AuthService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAuthService(db *gorm.DB, cfg *config.Config) *AuthService {
	return &AuthService{db: db, cfg: cfg}
}

func (s *AuthService) Register(req models.RegisterRequest) (*models.User, error) {
	// M5: Validate email format
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRegex.MatchString(req.Email) {
		return nil, errors.New("invalid email address format")
	}

	// M2: Enforce password strength
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	// M1: Determine role — first admin registers freely (bootstrap);
	// subsequent public registrations are blocked.
	// Admins can still create users via the admin panel (POST /api/v1/users).
	var adminCount int64
	s.db.Model(&models.User{}).Where("role = ?", "admin").Count(&adminCount)
	if adminCount > 0 {
		return nil, errors.New("registration is closed. Contact your administrator to create an account")
	}

	var existing models.User
	if err := s.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return nil, errors.New("email already registered")
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	apiKey, _ := utils.GenerateAPIKey()

	user := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Name:         req.Name,
		Role:         "admin", // first user is always admin (bootstrap)
		APIKey:       apiKey,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) Login(req models.LoginRequest) (string, *models.User, bool, error) {
	var user models.User
	if err := s.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return "", nil, false, errors.New("invalid credentials")
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return "", nil, false, errors.New("invalid credentials")
	}

	// If 2FA is enabled, don't issue token yet — require OTP
	if user.TwoFactorEnabled {
		return "", &user, true, nil
	}

	token, err := utils.GenerateJWT(user.ID, user.Email, user.Role, s.cfg.JWT.Secret, s.cfg.JWT.Expiration)
	if err != nil {
		return "", nil, false, err
	}

	return token, &user, false, nil
}

// Validate2FA validates OTP and issues a JWT token
func (s *AuthService) Validate2FA(req models.Validate2FARequest) (string, *models.User, error) {
	var user models.User
	if err := s.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	if !utils.CheckPassword(req.Password, user.PasswordHash) {
		return "", nil, errors.New("invalid credentials")
	}

	if !user.TwoFactorEnabled || user.TwoFactorSecret == "" {
		return "", nil, errors.New("2FA is not enabled")
	}

	// First try TOTP validation
	valid := totp.Validate(req.Code, user.TwoFactorSecret)

	// If not valid, try recovery codes
	if !valid {
		valid = s.useRecoveryCode(user.ID, req.Code)
	}

	if !valid {
		return "", nil, errors.New("invalid 2FA code")
	}

	token, err := utils.GenerateJWT(user.ID, user.Email, user.Role, s.cfg.JWT.Secret, s.cfg.JWT.Expiration)
	if err != nil {
		return "", nil, err
	}

	return token, &user, nil
}

// Enable2FA generates a TOTP secret and recovery codes
func (s *AuthService) Enable2FA(userID uint64) (*models.TwoFactorSetup, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, errors.New("user not found")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "WP Manage",
		AccountName: user.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP: %w", err)
	}

	// Save the secret (but don't enable yet — wait for verification)
	s.db.Model(&user).Update("two_factor_secret", key.Secret())

	// Generate 10 recovery codes
	codes := make([]string, 10)
	for i := range codes {
		b := make([]byte, 4)
		rand.Read(b)
		codes[i] = hex.EncodeToString(b)
	}

	// Store hashed recovery codes
	s.db.Where("user_id = ?", userID).Delete(&models.RecoveryCode{})
	for _, code := range codes {
		hash, _ := utils.HashPassword(code)
		s.db.Create(&models.RecoveryCode{
			UserID:   userID,
			CodeHash: hash,
		})
	}

	return &models.TwoFactorSetup{
		Secret:        key.Secret(),
		QRCodeURI:     key.URL(),
		RecoveryCodes: codes,
	}, nil
}

// Verify2FA verifies a TOTP code and activates 2FA
func (s *AuthService) Verify2FA(userID uint64, code string) error {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return errors.New("user not found")
	}

	if user.TwoFactorSecret == "" {
		return errors.New("2FA setup not initiated")
	}

	if !totp.Validate(code, user.TwoFactorSecret) {
		return errors.New("invalid verification code")
	}

	return s.db.Model(&user).Update("two_factor_enabled", true).Error
}

// Disable2FA turns off 2FA for a user
func (s *AuthService) Disable2FA(userID uint64) error {
	s.db.Where("user_id = ?", userID).Delete(&models.RecoveryCode{})
	return s.db.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"two_factor_enabled": false,
		"two_factor_secret":  "",
	}).Error
}

func (s *AuthService) useRecoveryCode(userID uint64, code string) bool {
	var codes []models.RecoveryCode
	s.db.Where("user_id = ? AND used = false", userID).Find(&codes)
	for _, rc := range codes {
		if utils.CheckPassword(code, rc.CodeHash) {
			s.db.Model(&rc).Update("used", true)
			return true
		}
	}
	return false
}

func (s *AuthService) GetUserByID(id uint64) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
