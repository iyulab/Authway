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
	"github.com/pquerna/otp"
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
	// Disable turns the second factor off. proof is a current authenticator
	// code or an unused recovery code: knowing who the user is (a token) is
	// one factor, and must not be enough to remove the other.
	Disable(userID uuid.UUID, proof string) error
	VerifyRecoveryCode(userID uuid.UUID, code string) (bool, error)
	// RegenerateRecoveryCodes replaces the recovery codes; proof as for Disable.
	RegenerateRecoveryCodes(userID uuid.UUID, proof string) (*RecoveryCodesResponse, error)
	GetStatus(userID uuid.UUID) (*MFAStatusResponse, error)
}

type service struct {
	db          *gorm.DB
	userService user.Service
	logger      *zap.Logger
	issuer      string
	cipher      crypto.Cipher
	// now is the clock codes are checked against.
	now func() time.Time
}

func NewService(db *gorm.DB, userService user.Service, logger *zap.Logger, issuer string, cipher crypto.Cipher) Service {
	return &service{db: db, userService: userService, logger: logger, issuer: issuer, cipher: cipher, now: time.Now}
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

// ErrAlreadyEnabled refuses a new setup while the second factor is on. The
// secret being set up would replace the one in use, and a setup needs nothing
// but the user's token — so the factor is turned off first, which needs proof.
var ErrAlreadyEnabled = apierror.NewPublic("MFA is already enabled; turn it off first to set up another authenticator")

// ErrProofRequired refuses a change to the second factor that does not come
// with a current authenticator code or an unused recovery code.
var ErrProofRequired = apierror.NewPublic("a current authenticator code or an unused recovery code is required")

func (s *service) SetupTOTP(userID uuid.UUID) (*TOTPSetupResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	if u.TOTPEnabled {
		return nil, ErrAlreadyEnabled
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
	if ok, err := s.acceptCode(userID, secret, code); err != nil {
		return nil, err
	} else if !ok {
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
	return s.acceptCode(userID, secret, code)
}

// totpPeriod is how long an authenticator code is current, in seconds.
const totpPeriod = 30

// acceptCode reports whether code is a code of secret that has not been
// accepted before. A code is good for its own time step and the one either
// side of it, to allow for clocks that differ — which is also how long a code
// read over a shoulder or caught in transit stays usable. Accepting it once
// ends that: the step it belongs to is recorded, and neither it nor an
// earlier one is accepted again (RFC 6238 section 5.2).
func (s *service) acceptCode(userID uuid.UUID, secret, code string) (bool, error) {
	step, ok := matchingStep(secret, code, s.now())
	if !ok {
		return false, nil
	}
	// One statement, so two requests carrying the same code cannot both win.
	used := s.db.Model(&user.User{}).
		Where("id = ? AND (totp_last_step IS NULL OR totp_last_step < ?)", userID, step).
		Update("totp_last_step", step)
	if used.Error != nil {
		return false, fmt.Errorf("failed to record the accepted code: %w", used.Error)
	}
	if used.RowsAffected == 0 {
		s.logger.Warn("TOTP code refused: already used", zap.String("user_id", userID.String()))
	}
	return used.RowsAffected == 1, nil
}

// matchingStep returns the time step code belongs to: the current one, or
// the one before or after it.
func matchingStep(secret, code string, at time.Time) (int64, bool) {
	exact := totp.ValidateOpts{Period: totpPeriod, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	for _, offset := range []int{0, -1, 1} {
		t := at.Add(time.Duration(offset*totpPeriod) * time.Second)
		if ok, err := totp.ValidateCustom(code, secret, t, exact); err == nil && ok {
			return t.Unix() / totpPeriod, true
		}
	}
	return 0, false
}

// proves reports whether proof is a current authenticator code or an unused
// recovery code of the user. A recovery code is used up by proving.
func (s *service) proves(userID uuid.UUID, proof string) (bool, error) {
	if proof == "" {
		return false, nil
	}
	if ok, err := s.Verify(userID, proof); err != nil || ok {
		return ok, err
	}
	ok, err := s.VerifyRecoveryCode(userID, proof)
	if err != nil {
		// No recovery codes left is simply "not proven".
		return false, nil
	}
	return ok, nil
}

func (s *service) Disable(userID uuid.UUID, proof string) error {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled {
		return apierror.NewPublic("MFA is not enabled")
	}
	if ok, err := s.proves(userID, proof); err != nil {
		return err
	} else if !ok {
		return ErrProofRequired
	}
	updates := map[string]any{"totp_enabled": false, "totp_secret": nil, "totp_verified_at": nil, "totp_last_step": nil, "recovery_codes": nil}
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
func (s *service) RegenerateRecoveryCodes(userID uuid.UUID, proof string) (*RecoveryCodesResponse, error) {
	u, err := s.userService.GetByID(userID)
	if err != nil {
		return nil, apierror.NewPublic("user not found")
	}
	if !u.TOTPEnabled {
		return nil, apierror.NewPublic("MFA is not enabled")
	}
	if ok, err := s.proves(userID, proof); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrProofRequired
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
	for i := range count {
		bytes := make([]byte, 10)
		if _, err := rand.Read(bytes); err != nil {
			return nil, nil, err
		}
		encoded := base32.StdEncoding.EncodeToString(bytes)[:12]
		code := fmt.Sprintf("%s-%s-%s", encoded[:4], encoded[4:8], encoded[8:12])
		codes[i] = code
		// Hash the same normalized form VerifyRecoveryCode compares against
		// (dashes stripped, uppercased) — hashing the dashed display form here
		// made every recovery code unverifiable, since normalizeCode strips
		// the dashes before hashing the user's input.
		hashes[i] = hashCode(normalizeCode(code))
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
