package models

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm/schema"
)

// Fields tagged `gorm:"serializer:secret"` are encrypted when written and
// decrypted when read, so code keeps using the plain value on the struct while
// the database (and every backup of it) only ever holds ciphertext.
//
// The serializer runs for Create / Save / Find / First and struct Updates. It
// does NOT run for map updates or raw SQL — those would write plaintext, so
// never update a secret field that way.

var secretKey []byte

// SetSecretKey installs the key used for secret fields. Call it once at
// startup, before the first query.
func SetSecretKey(key []byte) {
	secretKey = key
}

func init() {
	schema.RegisterSerializer("secret", secretSerializer{})
}

type secretSerializer struct{}

func (secretSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var stored string
	switch v := dbValue.(type) {
	case nil:
	case []byte:
		stored = string(v)
	case string:
		stored = v
	default:
		return fmt.Errorf("secret field %s: unsupported database type %T", field.Name, dbValue)
	}

	plain := stored
	if utils.IsEncryptedSecret(stored) {
		var err error
		if plain, err = utils.DecryptSecret(secretKey, stored); err != nil {
			// The row stays readable (so it can be listed and deleted) but the
			// secret is gone: the provider must be re-added. Happens when the
			// key material in the environment was changed.
			slog.Error("Stored secret could not be decrypted — was SECRETS_KEY / JWT_SECRET changed?",
				"table", field.Schema.Table, "column", field.DBName, "error", err)
			plain = ""
		}
	}
	field.ReflectValueOf(ctx, dst).SetString(plain)
	return nil
}

func (secretSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	plain, _ := fieldValue.(string)
	if utils.IsEncryptedSecret(plain) {
		return plain, nil // already ciphertext: never encrypt twice
	}
	if len(secretKey) == 0 {
		return nil, fmt.Errorf("secret field %s: no encryption key configured", field.Name)
	}
	return utils.EncryptSecret(secretKey, plain)
}

// MarshalJSON hides the second credential of providers that keep a secret in
// APIEmail (Route53: secret access key, GoDaddy: API secret). Only Cloudflare
// stores an actual e-mail address there. APIKey is never serialised at all.
func (p DNSProvider) MarshalJSON() ([]byte, error) {
	type plain DNSProvider // drops this method, avoiding recursion
	out := plain(p)
	if out.ProviderType != "cloudflare" {
		out.APIEmail = ""
	}
	return json.Marshal(out)
}
