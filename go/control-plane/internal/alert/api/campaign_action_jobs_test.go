package api

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type campaignTransactionStub struct {
	committed  bool
	rolledBack bool
}

func (t *campaignTransactionStub) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (t *campaignTransactionStub) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return &sql.Row{}
}

func (t *campaignTransactionStub) Commit() error {
	t.committed = true
	return nil
}

func (t *campaignTransactionStub) Rollback() error {
	t.rolledBack = true
	return nil
}

func TestRunCampaignActionTransactionRollsBackWhenAuditFails(t *testing.T) {
	tx := &campaignTransactionStub{}
	auditErr := errors.New("audit insert failed")

	err := runCampaignActionTransaction(
		tx,
		func(campaignTransaction) error { return nil },
		func(campaignTransaction) error { return auditErr },
	)

	require.ErrorIs(t, err, auditErr)
	require.False(t, tx.committed)
	require.True(t, tx.rolledBack)
}

func TestRunCampaignActionTransactionCommitsBothRecords(t *testing.T) {
	tx := &campaignTransactionStub{}
	jobRecorded := false
	auditRecorded := false

	err := runCampaignActionTransaction(
		tx,
		func(campaignTransaction) error { jobRecorded = true; return nil },
		func(campaignTransaction) error { auditRecorded = true; return nil },
	)

	require.NoError(t, err)
	require.True(t, jobRecorded)
	require.True(t, auditRecorded)
	require.True(t, tx.committed)
	require.True(t, tx.rolledBack, "deferred rollback must release transaction resources after commit")
}
