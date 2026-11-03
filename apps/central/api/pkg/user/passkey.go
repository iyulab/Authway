package user

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Passkey is one WebAuthn credential an account signs in with, as a row of
// user_passkeys. An account may have several. Within a tenant a credential id
// names one passkey (unique index uq_user_passkeys_tenant_credential).
type Passkey struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null"`
	UserID       uuid.UUID `gorm:"type:uuid;not null"`
	CredentialID []byte    `gorm:"not null"`
	PublicKey    []byte    `gorm:"not null"`
	// SignCount is the signature counter the device reported at its last use.
	SignCount int64  `gorm:"not null"`
	AAGUID    []byte `gorm:"column:aaguid;not null"`
	// Transports are the authenticator transports the device offered, comma
	// separated, in the form WebAuthn names them ("internal", "hybrid").
	Transports string `gorm:"not null"`
	// Flags is the authenticator-data flags byte of the last ceremony.
	Flags int16 `gorm:"not null"`
	// Name is what the user calls the passkey.
	Name       string     `gorm:"not null"`
	CreatedAt  time.Time  `gorm:"not null"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"`
}

// TableName is the table the passkeys live in.
func (Passkey) TableName() string { return "user_passkeys" }

// TransportList is Transports as a list.
func (p Passkey) TransportList() []string {
	if p.Transports == "" {
		return nil
	}
	return strings.Split(p.Transports, ",")
}

// ErrPasskeyNotFound is a passkey the account does not have.
var ErrPasskeyNotFound = errors.New("the account has no such passkey")

// ErrPasskeyTaken refuses to register a credential id that already names a
// passkey in the tenant.
var ErrPasskeyTaken = errors.New("this passkey is already registered")

// PasskeyService keeps the passkeys of accounts.
type PasskeyService interface {
	// List returns the account's passkeys, oldest first.
	List(userID uuid.UUID) ([]Passkey, error)
	// Add stores a new passkey. ErrPasskeyTaken when the tenant already has a
	// passkey with the credential id.
	Add(p *Passkey) error
	// Remove deletes one of the account's passkeys. ErrPasskeyNotFound when the
	// account has no such passkey; ErrLastSignInMethod when it would leave the
	// account no way to sign in.
	Remove(userID, passkeyID uuid.UUID) error
	// Find returns the tenant's passkey with the credential id. ErrPasskeyNotFound
	// when there is none.
	Find(tenantID uuid.UUID, credentialID []byte) (*Passkey, error)
	// RecordUse stores what the device reported when the passkey signed in:
	// the signature counter and the flags byte.
	RecordUse(id uuid.UUID, signCount uint32, flags byte) error
}

type passkeyService struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewPasskeyService returns a PasskeyService over db.
func NewPasskeyService(db *gorm.DB, logger *zap.Logger) PasskeyService {
	return &passkeyService{db: db, logger: logger}
}

func (s *passkeyService) List(userID uuid.UUID) ([]Passkey, error) {
	var rows []Passkey
	if err := s.db.Where("user_id = ?", userID).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}
	return rows, nil
}

func (s *passkeyService) Add(p *Passkey) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	// A nil slice is stored as NULL, which the column refuses.
	if p.AAGUID == nil {
		p.AAGUID = []byte{}
	}
	err := s.db.Create(p).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrPasskeyTaken
	}
	if err != nil {
		return fmt.Errorf("failed to store passkey: %w", err)
	}
	s.logger.Info("Passkey registered", zap.String("user_id", p.UserID.String()), zap.String("passkey_id", p.ID.String()))
	return nil
}

func (s *passkeyService) Remove(userID, passkeyID uuid.UUID) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockAccount(tx, userID); err != nil {
			return err
		}
		var p Passkey
		if err := tx.Where("id = ? AND user_id = ?", passkeyID, userID).First(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPasskeyNotFound
			}
			return err
		}
		if err := requireAnotherWayToSignIn(tx, userID, 0, 1); err != nil {
			return err
		}
		return tx.Delete(&p).Error
	})
	if err != nil {
		if errors.Is(err, ErrPasskeyNotFound) || errors.Is(err, ErrLastSignInMethod) {
			return err
		}
		return fmt.Errorf("failed to remove passkey: %w", err)
	}
	s.logger.Info("Passkey removed", zap.String("user_id", userID.String()), zap.String("passkey_id", passkeyID.String()))
	return nil
}

func (s *passkeyService) Find(tenantID uuid.UUID, credentialID []byte) (*Passkey, error) {
	var p Passkey
	err := s.db.Where("tenant_id = ? AND credential_id = ?", tenantID, credentialID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPasskeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find passkey: %w", err)
	}
	return &p, nil
}

func (s *passkeyService) RecordUse(id uuid.UUID, signCount uint32, flags byte) error {
	// Two sign-ins at once both read the counter before either stored its own:
	// whichever stores last would put the number back. The counter only rises,
	// so a copy that replays the higher one is still told from the device.
	res := s.db.Model(&Passkey{}).Where("id = ?", id).Updates(map[string]any{
		"sign_count":   gorm.Expr("GREATEST(sign_count, ?)", int64(signCount)),
		"flags":        int16(flags),
		"last_used_at": time.Now(),
	})
	if res.Error != nil {
		return fmt.Errorf("failed to record passkey use: %w", res.Error)
	}
	return nil
}

// lockAccount holds the account's row until the transaction ends, so two
// changes to how one account signs in are made one after the other: each sees
// what the other left.
func lockAccount(tx *gorm.DB, userID uuid.UUID) error {
	var locked User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&locked).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrUserNotFound
	}
	return err
}

// requireAnotherWayToSignIn refuses with ErrLastSignInMethod unless the account
// is left a way to sign in after dropping that many of its identities and that
// many of its passkeys. A password, each identity and each passkey is one way.
// The caller holds the account's lock (lockAccount).
func requireAnotherWayToSignIn(tx *gorm.DB, userID uuid.UUID, dropIdentities, dropPasskeys int64) error {
	var hash string
	if err := tx.Model(&User{}).Where("id = ?", userID).Select("password_hash").Scan(&hash).Error; err != nil {
		return err
	}
	var identities, passkeys int64
	if err := tx.Model(&LinkedIdentity{}).Where("user_id = ?", userID).Count(&identities).Error; err != nil {
		return err
	}
	if err := tx.Model(&Passkey{}).Where("user_id = ?", userID).Count(&passkeys).Error; err != nil {
		return err
	}
	left := identities - dropIdentities + passkeys - dropPasskeys
	if hash != "" {
		left++
	}
	if left < 1 {
		return ErrLastSignInMethod
	}
	return nil
}
