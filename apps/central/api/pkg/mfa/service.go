package mfa

import (
	"authway/apps/central/api/pkg/apierror"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"authway/apps/central/api/pkg/crypto"
	"authway/apps/central/api/pkg/user"
)

// Service provides MFA/TOTP functionality
type Service interface {
	SetupTOTP(userID uuid.UUID) (*TOTPSetupResponse, error)
	VerifyAndEnable(userID uuid.UUID, code string) (*RecoveryCodesResponse, error)
	Verify(userID uuid.UUID, code string) (bool, error)
	Disable(userID uuid.UUID) error
	VerifyRecoveryCode(userID uuid.UUID, code string) (bool, error)
	RegenerateRecoveryCodes(userID uuid.UUID) (*RecoveryCodesResponse, error)
	GetStatus(userID uuid.UUID) (*MFAStatusResponse, error)
}

type service struct {
	db          *gorm.DB
	userService user.Service
	logger      *zap.Logger
	issuer      string
	cipher      crypto.Cipher
}

func NewService(db *gorm.DB, userService user.Service, logger *zap.Logger, issuer string, cipher crypto.Cipher) Service {
	return &service{db: db, userService: userService, logger: logger, issuer: issuer, cipher: cipher}
}

type TOTPSetupResponse struct {
	Secret        string `json:"secret"`
	QRCodeDataURL string `json:"qr_code_data_url"`
	Issuer        string `json:"issuer"`
	AccountName   string `json:"account_name"`
}

type RecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
	Message       string   `json:"message"`
}

type MFAStatusResponse struct {
	Enabled           bool       `json:"enabled"`
	EnabledAt         *time.Time `json:"enabled_at,omitempty"`
	RecoveryCodesLeft int        `json:"recovery_codes_left"`
}

func (s *service) SetupTOTP(userID uuid.UUID) (*TOTPSetupResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.issuer, AccountName: u.Email, SecretSize: 32})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}
	secret := key.Secret()
	encSecret, err := s.cipher.Encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt TOTP secret: %w", err)
	}
	if err := s.db.Model(&user.User{}).Where("id = ?", userID).Update("totp_secret", encSecret).Error; err != nil {
		return nil, fmt.Errorf("failed to store TOTP secret: %w", err)
	}
	// Use the otpauth URL produced by the totp library — it URL-encodes the
	// label (issuer:email) and parameters per the Key URI Format spec. The
	// previous fmt.Sprintf path silently broke for any email containing `+`
	// or `@` past the first occurrence (e.g. `user+alias@example.com`).
	qrCodeURL := key.URL()
	s.logger.Info("TOTP setup initiated", zap.String("user_id", userID.String()), zap.String("email", u.Email))
	return &TOTPSetupResponse{Secret: secret, QRCodeDataURL: qrCodeURL, Issuer: s.issuer, AccountName: u.Email}, nil
}
func (s *service) VerifyAndEnable(userID uuid.UUID, code string) (*RecoveryCodesResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	if u.TOTPSecret == nil || *u.TOTPSecret == "" {
		return nil, apierror.NewPublic("TOTP not set up for this user")
	}
	if u.TOTPEnabled {
		return nil, apierror.NewPublic("MFA is already enabled")
	}
	secret, err := s.cipher.Decrypt(*u.TOTPSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}
	if !totp.Validate(code, secret) {
		s.logger.Warn("Invalid TOTP code during setup", zap.String("user_id", userID.String()))
		return nil, apierror.NewPublic("invalid TOTP code")
	}
	recoveryCodes, hashedCodes, err := generateRecoveryCodes(8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recovery codes: %w", err)
	}
	codesJSON, _ := json.Marshal(hashedCodes)
	now := time.Now()
	updates := map[string]any{"totp_enabled": true, "totp_verified_at": now, "recovery_codes": string(codesJSON)}
	if err := s.db.Model(&user.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to enable MFA: %w", err)
	}
	s.logger.Info("MFA enabled", zap.String("user_id", userID.String()))
	return &RecoveryCodesResponse{RecoveryCodes: recoveryCodes, Message: "MFA enabled. Save recovery codes securely."}, nil
}
func (s *service) Verify(userID uuid.UUID, code string) (bool, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return false, apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled || u.TOTPSecret == nil {
		return false, apierror.NewPublic("MFA is not enabled")
	}
	secret, err := s.cipher.Decrypt(*u.TOTPSecret)
	if err != nil {
		return false, fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}
	return totp.Validate(code, secret), nil
}

func (s *service) Disable(userID uuid.UUID) error {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled {
		return apierror.NewPublic("MFA is not enabled")
	}
	updates := map[string]any{"totp_enabled": false, "totp_secret": nil, "totp_verified_at": nil, "recovery_codes": nil}
	if err := s.db.Model(&user.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to disable MFA: %w", err)
	}
	s.logger.Info("MFA disabled", zap.String("user_id", userID.String()))
	return nil
}
func (s *service) VerifyRecoveryCode(userID uuid.UUID, code string) (bool, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return false, apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled {
		return false, apierror.NewPublic("MFA is not enabled")
	}
	if u.RecoveryCodes == nil || *u.RecoveryCodes == "" {
		return false, apierror.NewPublic("no recovery codes")
	}
	var hashedCodes []string
	if err := json.Unmarshal([]byte(*u.RecoveryCodes), &hashedCodes); err != nil {
		return false, fmt.Errorf("failed to parse codes: %w", err)
	}
	inputHash := hashCode(normalizeCode(code))
	foundIndex := -1
	for i, hashed := range hashedCodes {
		if hashed == inputHash {
			foundIndex = i
			break
		}
	}
	if foundIndex == -1 {
		return false, nil
	}
	hashedCodes = append(hashedCodes[:foundIndex], hashedCodes[foundIndex+1:]...)
	codesJSON, _ := json.Marshal(hashedCodes)
	if err := s.db.Model(&user.User{}).Where("id = ?", userID).Update("recovery_codes", string(codesJSON)).Error; err != nil {
		return false, fmt.Errorf("failed to update codes: %w", err)
	}
	s.logger.Info("Recovery code used", zap.String("user_id", userID.String()), zap.Int("remaining", len(hashedCodes)))
	return true, nil
}
func (s *service) RegenerateRecoveryCodes(userID uuid.UUID) (*RecoveryCodesResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled {
		return nil, apierror.NewPublic("MFA is not enabled")
	}
	recoveryCodes, hashedCodes, err := generateRecoveryCodes(8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate codes: %w", err)
	}
	codesJSON, _ := json.Marshal(hashedCodes)
	if err := s.db.Model(&user.User{}).Where("id = ?", userID).Update("recovery_codes", string(codesJSON)).Error; err != nil {
		return nil, fmt.Errorf("failed to store codes: %w", err)
	}
	return &RecoveryCodesResponse{RecoveryCodes: recoveryCodes, Message: "New recovery codes generated."}, nil
}

func (s *service) GetStatus(userID uuid.UUID) (*MFAStatusResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	status := &MFAStatusResponse{Enabled: u.TOTPEnabled, EnabledAt: u.TOTPVerifiedAt}
	if u.TOTPEnabled && u.RecoveryCodes != nil && *u.RecoveryCodes != "" {
		var hashedCodes []string
		if err := json.Unmarshal([]byte(*u.RecoveryCodes), &hashedCodes); err == nil {
			status.RecoveryCodesLeft = len(hashedCodes)
		}
	}
	return status, nil
}
func generateRecoveryCodes(count int) ([]string, []string, error) {
	codes := make([]string, count)
	hashes := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 10)
		if _, err := rand.Read(bytes); err != nil {
			return nil, nil, err
		}
		encoded := base32.StdEncoding.EncodeToString(bytes)[:12]
		code := fmt.Sprintf("%s-%s-%s", encoded[:4], encoded[4:8], encoded[8:12])
		codes[i] = code
		hashes[i] = hashCode(code)
	}
	return codes, hashes, nil
}

func hashCode(code string) string {
	hash := sha256.Sum256([]byte(code))
	return hex.EncodeToString(hash[:])
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(code, "-", ""))
}
