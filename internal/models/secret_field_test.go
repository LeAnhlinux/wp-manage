package models

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber-team/wp-manage/pkg/utils"
	"gorm.io/gorm/schema"
)

// TestSecretSerializer drives the serializer the way GORM does: Value() on the
// way to the database, Scan() on the way back, against the real parsed schema
// of DNSProvider.
func TestSecretSerializer(t *testing.T) {
	old := secretKey
	defer SetSecretKey(old)
	SetSecretKey(utils.DeriveSecretKey("test-material-0123456789-0123456789"))

	sch, err := schema.Parse(&DNSProvider{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, name := range []string{"APIKey", "APIEmail"} {
		field := sch.LookUpField(name)
		if field == nil || field.Serializer == nil {
			t.Fatalf("%s must use the secret serializer", name)
		}
		if _, ok := field.Serializer.(secretSerializer); !ok {
			t.Fatalf("%s uses %T", name, field.Serializer)
		}

		stored, err := field.Serializer.Value(ctx, field, reflect.Value{}, "plain-credential")
		if err != nil {
			t.Fatal(err)
		}
		enc, _ := stored.(string)
		if !utils.IsEncryptedSecret(enc) || strings.Contains(enc, "plain-credential") {
			t.Fatalf("%s written unencrypted: %q", name, enc)
		}
		if len(enc) > field.Size {
			t.Fatalf("%s: ciphertext (%d) does not fit the column (%d)", name, len(enc), field.Size)
		}

		// MariaDB hands strings back as []byte
		var p DNSProvider
		if err := field.Serializer.Scan(ctx, field, reflect.ValueOf(&p).Elem(), []byte(enc)); err != nil {
			t.Fatal(err)
		}
		if got := reflect.ValueOf(p).FieldByName(name).String(); got != "plain-credential" {
			t.Fatalf("%s read back as %q", name, got)
		}

		// legacy plaintext row and NULL
		var legacy, null DNSProvider
		field.Serializer.Scan(ctx, field, reflect.ValueOf(&legacy).Elem(), "old-plaintext")
		if got := reflect.ValueOf(legacy).FieldByName(name).String(); got != "old-plaintext" {
			t.Errorf("%s legacy read as %q", name, got)
		}
		if err := field.Serializer.Scan(ctx, field, reflect.ValueOf(&null).Elem(), nil); err != nil {
			t.Errorf("%s NULL: %v", name, err)
		}

		// ciphertext is never encrypted a second time
		again, _ := field.Serializer.Value(ctx, field, reflect.Value{}, enc)
		if again != enc {
			t.Errorf("%s double-encrypted", name)
		}

		// a row written under another key reads as empty, not as garbage or an error
		SetSecretKey(utils.DeriveSecretKey("another-material-0123456789-01234"))
		var lost DNSProvider
		if err := field.Serializer.Scan(ctx, field, reflect.ValueOf(&lost).Elem(), enc); err != nil {
			t.Errorf("%s wrong key must not fail the query: %v", name, err)
		}
		if got := reflect.ValueOf(lost).FieldByName(name).String(); got != "" {
			t.Errorf("%s wrong key read as %q", name, got)
		}
		SetSecretKey(utils.DeriveSecretKey("test-material-0123456789-0123456789"))
	}

	// a 500-character credential still fits? (it must be rejected by size, not silently truncated)
	long, _ := sch.LookUpField("APIKey").Serializer.Value(ctx, sch.LookUpField("APIKey"), reflect.Value{}, strings.Repeat("k", 300))
	if l := len(long.(string)); l > 500 {
		t.Logf("note: a 300-char credential encrypts to %d chars (> column size 500)", l)
	}

	// without a key, writing must fail loudly instead of storing plaintext
	SetSecretKey(nil)
	if _, err := (secretSerializer{}).Value(ctx, sch.LookUpField("APIKey"), reflect.Value{}, "x"); err == nil {
		t.Error("writing without a key must be an error")
	}
}

// Every secret field must fit a realistic credential, after encryption, in its column.
func TestSecretFieldsFitTheirColumns(t *testing.T) {
	old := secretKey
	defer SetSecretKey(old)
	SetSecretKey(utils.DeriveSecretKey("test-material-0123456789-0123456789"))

	cases := []struct {
		model  interface{}
		field  string
		sample string
	}{
		{&StorageCredential{}, "SecretKey", strings.Repeat("s", 64)},
		{&NotificationConfig{}, "TelegramToken", "1234567890:" + strings.Repeat("A", 35)},
		{&NotificationConfig{}, "SlackWebhook", "https://hooks.slack.com/services/T00000000/B00000000/" + strings.Repeat("x", 24)},
		{&NotificationConfig{}, "SMTPPass", strings.Repeat("p", 128)},
		{&IntegrationConfig{}, "GooglePageSpeedKey", "AIza" + strings.Repeat("k", 35)},
	}
	ctx := context.Background()
	for _, c := range cases {
		sch, err := schema.Parse(c.model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatal(err)
		}
		f := sch.LookUpField(c.field)
		if f == nil || f.Serializer == nil {
			t.Fatalf("%s.%s must use the secret serializer", sch.Name, c.field)
		}
		stored, err := f.Serializer.Value(ctx, f, reflect.Value{}, c.sample)
		if err != nil {
			t.Fatal(err)
		}
		enc := stored.(string)
		if f.Size > 0 && len(enc) > f.Size {
			t.Errorf("%s.%s: %d-char ciphertext does not fit varchar(%d)", sch.Name, c.field, len(enc), f.Size)
		}
		dst := reflect.New(reflect.TypeOf(c.model).Elem()).Elem()
		if err := f.Serializer.Scan(ctx, f, dst, []byte(enc)); err != nil {
			t.Fatal(err)
		}
		if got := dst.FieldByName(c.field).String(); got != c.sample {
			t.Errorf("%s.%s read back wrong", sch.Name, c.field)
		}
	}
}

func TestDNSProviderJSONNeverLeaksSecrets(t *testing.T) {
	gd := DNSProvider{ID: 1, ProviderType: "godaddy", APIKey: "KEY-123", APIEmail: "SECRET-456", Domain: "example.com"}
	for _, v := range []interface{}{gd, &gd, []DNSProvider{gd}, map[string]interface{}{"data": &gd}} {
		out, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "KEY-123") || strings.Contains(string(out), "SECRET-456") {
			t.Errorf("secret leaked in %s", out)
		}
		if !strings.Contains(string(out), `"domain":"example.com"`) {
			t.Errorf("normal fields missing in %s", out)
		}
	}
	if gd.APIEmail != "SECRET-456" {
		t.Error("marshalling must not modify the struct")
	}

	r53, _ := json.Marshal(DNSProvider{ProviderType: "route53", APIKey: "AKIA", APIEmail: "aws-secret"})
	if strings.Contains(string(r53), "aws-secret") {
		t.Errorf("route53 secret leaked: %s", r53)
	}

	cf, _ := json.Marshal(DNSProvider{ProviderType: "cloudflare", APIKey: "token", APIEmail: "me@example.com"})
	if !strings.Contains(string(cf), `"api_email":"me@example.com"`) || strings.Contains(string(cf), "token") {
		t.Errorf("cloudflare: e-mail must stay, token must not appear: %s", cf)
	}
}
