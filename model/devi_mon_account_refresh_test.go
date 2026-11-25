package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviMonAccountRefreshDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useSystemTaskMatrixDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&DeviMonAccountRefresh{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&DeviMonAccountRefresh{})) })
			for _, row := range []DeviMonAccountRefresh{{Email: "account@example.test", StartedAtUS: 5, Data: "first"}, {Email: "account@example.test", StartedAtUS: 15, Data: "newest"}, {Email: "account@example.test", StartedAtUS: 10, Data: "older request finished last"}, {Email: "other@example.test", StartedAtUS: 6, Data: "other"}} {
				require.NoError(t, SaveDeviMonAccountRefresh(context.Background(), &row))
			}
			require.NoError(t, DeleteDeviMonRefreshesBefore(15))
			rows, err := GetDeviMonRefreshesSince(0)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "newest", rows[0].Data)
		})
	}
}
