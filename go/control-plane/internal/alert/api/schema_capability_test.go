package api

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func requiredColumnRows(required map[string][]string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"table_name", "column_name"})
	for table, columns := range required {
		for _, column := range columns {
			rows.AddRow(table, column)
		}
	}
	return rows
}

func TestVerifyCampaignWorkbenchSchemaAcceptsCompleteCapabilities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(requiredPostgresColumnsQuery)).
		WillReturnRows(requiredColumnRows(campaignWorkbenchRequiredColumns))

	require.NoError(t, verifyCampaignWorkbenchSchema(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyCampaignAggregateV2SchemaAcceptsCompleteCapabilities(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(requiredPostgresColumnsQuery)).
		WillReturnRows(requiredColumnRows(campaignAggregateV2RequiredColumns))

	require.NoError(t, verifyCampaignAggregateV2Schema(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyTopicGovernanceSchemaRejectsMissingCapability(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	required := map[string][]string{}
	for table, columns := range topicGovernanceRequiredColumns {
		required[table] = append([]string(nil), columns...)
	}
	required["topic_actions"] = required["topic_actions"][:len(required["topic_actions"])-1]
	mock.ExpectQuery(regexp.QuoteMeta(requiredPostgresColumnsQuery)).
		WillReturnRows(requiredColumnRows(required))

	err = verifyTopicGovernanceSchema(context.Background(), db)
	require.ErrorContains(t, err, "topic_actions.lease_until")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyRequiredPostgresColumnsRejectsUnconfiguredDatabase(t *testing.T) {
	err := verifyRequiredPostgresColumns(
		context.Background(),
		nil,
		map[string][]string{"required_table": {"required_column"}},
	)
	require.ErrorContains(t, err, "postgres is not configured")
}
