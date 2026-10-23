package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRC46SchemaUpgradeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_MYSQL_DSN")
			if dialect == "postgres" {
				dsn = os.Getenv("TEST_POSTGRES_DSN")
			}
			if dialect == "sqlite" {
				dsn = "local"
				previous := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "rc46.db")
				t.Cleanup(func() { common.SQLitePath = previous })
			}
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("RC46_TEST_DSN", dsn)
			db, _, err := chooseDB("RC46_TEST_DSN", false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			for _, tc := range []struct {
				name  string
				row   any
				added map[string]bool
			}{
				{"tokens", &Token{Id: 1, UserId: 7, Key: "rc46-fixture", Name: "existing", RemainQuota: 12345}, map[string]bool{"AllowUserQuotaQuery": true}},
				{"quota_data", &QuotaData{Id: 1, UserID: 7, Username: "existing", ModelName: "fixture", Quota: 12345, TokenUsed: 23}, map[string]bool{"CacheReadTokens": true, "CacheCreationTokens": true}},
			} {
				for _, upgrade := range []bool{false, true} {
					name := tc.name + "/fresh"
					if upgrade {
						name = tc.name + "/upgrade"
					}
					t.Run(name, func(t *testing.T) {
						table := "rc46_migration_" + tc.name
						t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(table)) })
						if upgrade {
							// These models differ from rc.44 only by the listed additive fields.
							typ := reflect.TypeOf(tc.row).Elem()
							var fields []reflect.StructField
							for i := range typ.NumField() {
								field := typ.Field(i)
								if !tc.added[field.Name] {
									fields = append(fields, field)
								}
							}
							old := reflect.New(reflect.StructOf(fields))
							for _, field := range fields {
								old.Elem().FieldByName(field.Name).Set(reflect.ValueOf(tc.row).Elem().FieldByName(field.Name))
							}
							require.NoError(t, db.Table(table).AutoMigrate(old.Interface()))
							require.NoError(t, db.Table(table).Create(old.Interface()).Error)
						}
						require.NoError(t, db.Table(table).AutoMigrate(tc.row))
						if !upgrade {
							require.NoError(t, db.Table(table).Create(tc.row).Error)
						}
						recorder := &migrationSQLRecorder{}
						require.NoError(t, db.Session(&gorm.Session{Logger: recorder}).Table(table).AutoMigrate(tc.row))
						require.Empty(t, recorder.schemaMutations(), "repeated startup must not mutate schema")
						stored := reflect.New(reflect.TypeOf(tc.row).Elem()).Interface()
						require.NoError(t, db.Table(table).First(stored, 1).Error)
						require.Equal(t, tc.row, stored, "existing data and zero-valued new fields must survive migration")
					})
				}
			}
		})
	}
}
