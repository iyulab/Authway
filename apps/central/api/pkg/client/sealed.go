package client

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/zap"
	"gorm.io/gorm/schema"

	"authway/apps/central/api/pkg/crypto"
)

// What a tenant hands Authway so that its users can sign in through another
// identity provider — the client secret it holds there, or its signing key —
// is somebody else's credential. Authway has to present it, so it cannot be
// hashed like a client's own secret; it is sealed at rest instead, and only
// ever held in the clear in memory.
//
// The sealing sits in the mapping between a Client and its row (the "sealed"
// serializer on the fields), not in the code that reads and writes clients:
// every query that loads a Client gets the secret in the clear and every
// write seals it, with nothing to remember at the call sites.

// sealedColumns are the columns the serializer is on.
var sealedColumns = []string{"google_client_secret", "github_client_secret", "microsoft_client_secret", "apple_private_key"}

// atRest seals those columns. Until SealSecretsWith is called it leaves
// values as they are — the state of a development setup with no key, and of
// tests that are not about this.
var atRest = mustCipher("")

func mustCipher(key string) crypto.Cipher {
	c, err := crypto.NewCipher(key)
	if err != nil {
		panic(err)
	}
	return c
}

// SealSecretsWith sets the cipher that seals provider secrets at rest. Call it
// once at startup, before any client is read or written.
func SealSecretsWith(c crypto.Cipher) {
	if c != nil {
		atRest = c
	}
}

func init() {
	schema.RegisterSerializer("sealed", sealedSerializer{})
}

// sealedSerializer maps a *string field to a column that holds it sealed. A
// value written before sealing was introduced is read as it is (crypto.Cipher
// passes an untagged value through), so sealing can be turned on over
// existing rows.
type sealedSerializer struct{}

func (sealedSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	target := field.ReflectValueOf(ctx, dst)
	var stored string
	switch v := dbValue.(type) {
	case nil:
		target.Set(reflect.Zero(field.FieldType))
		return nil
	case string:
		stored = v
	case []byte:
		stored = string(v)
	default:
		return fmt.Errorf("client: %s holds a %T, not text", field.DBName, dbValue)
	}
	if stored != "" {
		opened, err := atRest.Decrypt(stored)
		if err != nil {
			return fmt.Errorf("client: %s cannot be read: %w", field.DBName, err)
		}
		stored = opened
	}
	target.Set(reflect.ValueOf(&stored))
	return nil
}

func (sealedSerializer) Value(_ context.Context, field *schema.Field, _ reflect.Value, fieldValue any) (any, error) {
	secret, ok := fieldValue.(*string)
	if !ok {
		return nil, fmt.Errorf("client: %s is a %T, not a *string", field.DBName, fieldValue)
	}
	if secret == nil {
		return nil, nil
	}
	if *secret == "" {
		return "", nil
	}
	return atRest.Encrypt(*secret)
}

// SealStoredSecrets seals the provider secrets that were stored before sealing
// was introduced, or while no key was configured. It does nothing without a
// key, and nothing to a value that is already sealed.
func SealStoredSecrets(svc Service, logger *zap.Logger) error {
	s, ok := svc.(*service)
	if !ok || !atRest.Enabled() {
		return nil
	}
	sealed := 0
	for _, column := range sealedColumns {
		// The rows as they are stored, not as clients: what is read here is
		// the value to seal. Deleted clients are included — their rows hold
		// the secret all the same.
		var pending []struct {
			ID    string
			Value string
		}
		if err := s.db.Raw(
			"SELECT id::text AS id, "+column+" AS value FROM clients WHERE "+column+" IS NOT NULL AND "+column+" <> '' AND "+column+" NOT LIKE ?",
			crypto.SealedPrefix+"%").Scan(&pending).Error; err != nil {
			return fmt.Errorf("failed to list clients with an unsealed %s: %w", column, err)
		}
		for _, row := range pending {
			value, err := atRest.Encrypt(row.Value)
			if err != nil {
				return fmt.Errorf("failed to seal %s: %w", column, err)
			}
			// Only the one column, and only if it still holds what was read:
			// the client itself has not been edited, and an edit made
			// meanwhile has already sealed its own value.
			if err := s.db.Exec("UPDATE clients SET "+column+" = ? WHERE id = ? AND "+column+" = ?", value, row.ID, row.Value).Error; err != nil {
				return fmt.Errorf("failed to seal %s: %w", column, err)
			}
			sealed++
		}
	}
	if sealed > 0 {
		logger.Info("Sealed provider secrets that were stored as they are", zap.Int("values", sealed))
	}
	return nil
}
