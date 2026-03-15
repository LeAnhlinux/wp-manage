package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/gofiber-team/wp-manage/internal/config"
	"github.com/gofiber-team/wp-manage/internal/models"
	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm"
)

const (
	mobileAccessTokenExpiry  = 15 * time.Minute
	mobileTempTokenExpiry    = 5 * time.Minute
	mobileRefreshTokenExpiry = 30 * 24 * time.Hour
)

// MobileAuthService manages mobile-specific authentication:
//   - Short-lived JWT access tokens (15 min)
//   - Long-lived opaque refresh tokens stored hashed in DB (30 days)
//   - Device registration for push notifications
type MobileAuthService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewMobileAuthService(db *gorm.DB, cfg *config.Config) *MobileAuthService {
	return &MobileAuthService{db: db, cfg: cfg}
}

// ─── Login ────────────────────────────────────────────────────────────────────

// LoginResult is returned by Login. If Requires2FA is true, TempToken is set
// and the caller must complete the flow via Complete2FA.
type LoginResult struct {
	Requires2FA bool
	TempToken   string // short-lived JWT valid only for /auth/2fa/complete
	TokenResp   *models.MobileTokenResponse
}

// Login validates credentials. Returns a full token pair when 2FA is off,
// or a temp token when 2FA is required.
func (s *MobileAuthService) Login(email, password string) (*LoginResult, error) {
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, errors.New("invalid credentials")
	}
	if !utils.CheckPassword(password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	if user.TwoFactorEnabled {
		// Issue a temp JWT with role "2fa_pending" — only /auth/2fa/complete accepts it
		tempToken, err := utils.GenerateJWT(user.ID, user.Email, "2fa_pending", s.cfg.JWT.Secret, mobileTempTokenExpiry)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Requires2FA: true, TempToken: tempToken}, nil
	}

	tokenResp, err := s.issueTokenPair(&user, "", "")
	if err != nil {
		return nil, err
	}
	return &LoginResult{TokenResp: tokenResp}, nil
}

// Complete2FA validates the TOTP code (or recovery code) and issues a full token pair.
func (s *MobileAuthService) Complete2FA(tempToken, code string) (*models.MobileTokenResponse, error) {
	claims, err := utils.ParseJWT(tempToken, s.cfg.JWT.Secret)
	if err != nil || claims.Role != "2fa_pending" {
		return nil, errors.New("invalid or expired 2FA session")
	}

	var user models.User
	if err := s.db.First(&user, claims.UserID).Error; err != nil {
		return nil, errors.New("user not found")
	}
	if !user.TwoFactorEnabled || user.TwoFactorSecret == "" {
		return nil, errors.New("2FA not enabled")
	}

	// Try TOTP
	valid := validateTOTP(code, user.TwoFactorSecret)
	// Fallback: recovery codes (reuse existing auth service logic via direct DB)
	if !valid {
		valid = s.useRecoveryCode(user.ID, code)
	}
	if !valid {
		return nil, errors.New("invalid 2FA code")
	}

	return s.issueTokenPair(&user, "", "")
}

// ─── Refresh ──────────────────────────────────────────────────────────────────

// Refresh validates a refresh token and issues a new access token.
// The refresh token itself is only rotated when it has less than 7 days remaining.
func (s *MobileAuthService) Refresh(rawRefreshToken string) (*models.MobileTokenResponse, error) {
	hash := hashToken(rawRefreshToken)

	var rt models.MobileRefreshToken
	if err := s.db.Preload("User").Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if time.Now().After(rt.ExpiresAt) {
		s.db.Delete(&rt)
		return nil, errors.New("refresh token expired")
	}

	// Update last_used_at
	now := time.Now()
	s.db.Model(&rt).Update("last_used_at", now)

	// Issue new access token
	accessToken, err := utils.GenerateJWT(
		rt.User.ID, rt.User.Email, rt.User.Role,
		s.cfg.JWT.Secret, mobileAccessTokenExpiry,
	)
	if err != nil {
		return nil, err
	}

	resp := &models.MobileTokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(mobileAccessTokenExpiry.Seconds()),
		User: models.MobileMe{
			ID:               rt.User.ID,
			Name:             rt.User.Name,
			Email:            rt.User.Email,
			Role:             rt.User.Role,
			TwoFactorEnabled: rt.User.TwoFactorEnabled,
		},
	}

	// Rotate refresh token when < 7 days remain
	if time.Until(rt.ExpiresAt) < 7*24*time.Hour {
		newRaw, newHash, err := generateRefreshToken()
		if err == nil {
			s.db.Model(&rt).Updates(map[string]interface{}{
				"token_hash": newHash,
				"expires_at": time.Now().Add(mobileRefreshTokenExpiry),
			})
			resp.RefreshToken = newRaw
			resp.RefreshExpiresIn = int(mobileRefreshTokenExpiry.Seconds())
		}
	}

	return resp, nil
}

// ─── Device Registration ──────────────────────────────────────────────────────

// RegisterDevice stores or updates the FCM/APNS token for a device.
func (s *MobileAuthService) RegisterDevice(userID uint64, req models.MobileDeviceRequest) error {
	var rt models.MobileRefreshToken
	// Find the most-recent refresh token for this device
	err := s.db.Where("user_id = ? AND device_id = ?", userID, req.DeviceID).
		Order("created_at DESC").First(&rt).Error
	if err != nil {
		// No session for this device yet — just store it against any active session
		return s.db.Model(&models.MobileRefreshToken{}).
			Where("user_id = ?", userID).
			Order("created_at DESC").
			Limit(1).
			Updates(map[string]interface{}{
				"device_id":   req.DeviceID,
				"device_type": req.DeviceType,
				"device_name": req.DeviceName,
				"fcm_token":   req.FCMToken,
				"apns_token":  req.APNSToken,
			}).Error
	}
	return s.db.Model(&rt).Updates(map[string]interface{}{
		"device_type": req.DeviceType,
		"device_name": req.DeviceName,
		"fcm_token":   req.FCMToken,
		"apns_token":  req.APNSToken,
	}).Error
}

// ─── Logout ───────────────────────────────────────────────────────────────────

// Logout invalidates a specific refresh token (device-level logout).
func (s *MobileAuthService) Logout(rawRefreshToken string) {
	hash := hashToken(rawRefreshToken)
	s.db.Where("token_hash = ?", hash).Delete(&models.MobileRefreshToken{})
}

// LogoutAll invalidates all refresh tokens for a user (global logout).
func (s *MobileAuthService) LogoutAll(userID uint64) {
	s.db.Where("user_id = ?", userID).Delete(&models.MobileRefreshToken{})
}

// ─── Notification center ─────────────────────────────────────────────────────

func (s *MobileAuthService) ListNotifications(userID uint64, page, perPage int) ([]models.MobileNotification, int64, error) {
	var items []models.MobileNotification
	var total int64
	offset := (page - 1) * perPage
	q := s.db.Model(&models.MobileNotification{}).Where("user_id = ?", userID)
	q.Count(&total)
	err := q.Order("sent_at DESC").Offset(offset).Limit(perPage).Find(&items).Error
	return items, total, err
}

func (s *MobileAuthService) MarkRead(userID, notifID uint64) error {
	now := time.Now()
	return s.db.Model(&models.MobileNotification{}).
		Where("id = ? AND user_id = ?", notifID, userID).
		Update("read_at", now).Error
}

func (s *MobileAuthService) MarkAllRead(userID uint64) error {
	now := time.Now()
	return s.db.Model(&models.MobileNotification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", now).Error
}

func (s *MobileAuthService) UnreadCount(userID uint64) int64 {
	var count int64
	s.db.Model(&models.MobileNotification{}).
		Where("user_id = ? AND read_at IS NULL", userID).Count(&count)
	return count
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *MobileAuthService) issueTokenPair(user *models.User, deviceID, deviceType string) (*models.MobileTokenResponse, error) {
	accessToken, err := utils.GenerateJWT(
		user.ID, user.Email, user.Role,
		s.cfg.JWT.Secret, mobileAccessTokenExpiry,
	)
	if err != nil {
		return nil, err
	}

	rawRefresh, refreshHash, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}

	rt := models.MobileRefreshToken{
		UserID:     user.ID,
		TokenHash:  refreshHash,
		DeviceID:   deviceID,
		DeviceType: deviceType,
		ExpiresAt:  time.Now().Add(mobileRefreshTokenExpiry),
	}
	if err := s.db.Create(&rt).Error; err != nil {
		return nil, err
	}

	return &models.MobileTokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     rawRefresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(mobileAccessTokenExpiry.Seconds()),
		RefreshExpiresIn: int(mobileRefreshTokenExpiry.Seconds()),
		User: models.MobileMe{
			ID:               user.ID,
			Name:             user.Name,
			Email:            user.Email,
			Role:             user.Role,
			TwoFactorEnabled: user.TwoFactorEnabled,
		},
	}, nil
}

func generateRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = hex.EncodeToString(b)
	hash = hashToken(raw)
	return
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *MobileAuthService) useRecoveryCode(userID uint64, code string) bool {
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

// validateTOTP reuses totp.Validate from the pquerna/otp package which is
// already imported in auth_service.go (same package).
func validateTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}
