package database

import (
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/gofiber-team/wp-manage/internal/models"
	"gorm.io/gorm/schema"
)

// The startup migration works on raw tables with hard-coded column names; the
// models declare the same fields with `serializer:secret`. If the two drift
// apart, a secret is either never migrated or migrated but read as ciphertext.
func TestStoredSecretColumnsMatchModels(t *testing.T) {
	all := []interface{}{
		&models.DNSProvider{}, &models.StorageCredential{},
		&models.NotificationConfig{}, &models.IntegrationConfig{},
	}
	fromModels := map[string][]string{}
	for _, m := range all {
		sch, err := schema.Parse(m, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range sch.Fields {
			if f.TagSettings["SERIALIZER"] == "secret" {
				fromModels[sch.Table] = append(fromModels[sch.Table], f.DBName)
			}
		}
	}
	norm := func(m map[string][]string) map[string][]string {
		out := map[string][]string{}
		for k, v := range m {
			c := append([]string(nil), v...)
			sort.Strings(c)
			out[k] = c
		}
		return out
	}
	if got, want := norm(storedSecretColumns), norm(fromModels); !reflect.DeepEqual(got, want) {
		t.Errorf("migration list and model tags differ:\n migration: %v\n models:    %v", got, want)
	}
	if dbString([]byte("abc")) != "abc" || dbString("x") != "x" || dbString(nil) != "" {
		t.Error("dbString")
	}
}
