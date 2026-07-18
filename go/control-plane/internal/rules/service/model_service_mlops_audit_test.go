package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	stderrors "errors"
	"sync"
	"testing"

	"go.uber.org/zap"
)

var errRequiredAuditUnavailable = stderrors.New("required audit store unavailable")
var errAuditIntentUpdateFailed = stderrors.New("audit intent update failed")

type failingAuditConnector struct{}

func (failingAuditConnector) Connect(context.Context) (driver.Conn, error) {
	return failingAuditConn{}, nil
}
func (failingAuditConnector) Driver() driver.Driver { return failingAuditDriver{} }

type failingAuditDriver struct{}

func (failingAuditDriver) Open(string) (driver.Conn, error) { return failingAuditConn{}, nil }

type failingAuditConn struct{}

func (failingAuditConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (failingAuditConn) Close() error                        { return nil }
func (failingAuditConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (failingAuditConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errRequiredAuditUnavailable
}

func TestRecordMLOpsWorkflowAuditFailsClosedWhenDatabaseInsertFails(t *testing.T) {
	db := sql.OpenDB(failingAuditConnector{})
	defer db.Close()
	service := NewModelService(db, nil, nil, nil, zap.NewNop(), DefaultModelServiceConfig())
	opCtx := &OperationContext{TenantID: "tenant-a", UserID: "user-a", Username: "tester"}

	err := service.RecordMLOpsWorkflowAudit(context.Background(), opCtx, "MLOPS_WORKFLOW_STOP_REQUESTED", "workflow-a", map[string]interface{}{"phase": "Running"}, nil)
	if err == nil {
		t.Fatal("required workflow audit insert failure must be returned to the handler")
	}
	if !stderrors.Is(err, errRequiredAuditUnavailable) {
		t.Fatalf("expected wrapped database failure, got %v", err)
	}
}

type transactionalAuditState struct {
	mu         sync.Mutex
	failUpdate bool
	execCount  int
	commits    int
	rollbacks  int
}

type transactionalAuditConnector struct{ state *transactionalAuditState }

func (c transactionalAuditConnector) Connect(context.Context) (driver.Conn, error) {
	return &transactionalAuditConn{state: c.state}, nil
}
func (c transactionalAuditConnector) Driver() driver.Driver {
	return transactionalAuditDriver{state: c.state}
}

type transactionalAuditDriver struct{ state *transactionalAuditState }

func (d transactionalAuditDriver) Open(string) (driver.Conn, error) {
	return &transactionalAuditConn{state: d.state}, nil
}

type transactionalAuditConn struct{ state *transactionalAuditState }

func (c *transactionalAuditConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *transactionalAuditConn) Close() error                        { return nil }
func (c *transactionalAuditConn) Begin() (driver.Tx, error) {
	c.state.mu.Lock()
	c.state.execCount = 0
	c.state.mu.Unlock()
	return &transactionalAuditTx{state: c.state}, nil
}
func (c *transactionalAuditConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.execCount++
	if c.state.failUpdate && c.state.execCount == 2 {
		return nil, errAuditIntentUpdateFailed
	}
	return driver.RowsAffected(1), nil
}

type transactionalAuditTx struct{ state *transactionalAuditState }

func (tx *transactionalAuditTx) Commit() error {
	tx.state.mu.Lock()
	tx.state.commits++
	tx.state.mu.Unlock()
	return nil
}
func (tx *transactionalAuditTx) Rollback() error {
	tx.state.mu.Lock()
	tx.state.rollbacks++
	tx.state.mu.Unlock()
	return nil
}

func TestRecordAutomatedMLOpsAuditCompletionRollsBackAndRetriesIdempotently(t *testing.T) {
	state := &transactionalAuditState{failUpdate: true}
	db := sql.OpenDB(transactionalAuditConnector{state: state})
	defer db.Close()
	svc := NewModelService(db, nil, nil, nil, zap.NewNop(), DefaultModelServiceConfig())
	opCtx := &OperationContext{TenantID: "tenant-a", Username: "reconciler"}
	complete := func() error {
		return svc.RecordAutomatedMLOpsAuditCompletion(
			context.Background(), opCtx, "MLOPS_AUTOMATED_RETRAIN_SUBMITTED",
			"mlops-fp-rate-r336", "mlops-intent-r336", map[string]interface{}{"trigger": "fp_rate"},
		)
	}

	if err := complete(); !stderrors.Is(err, errAuditIntentUpdateFailed) {
		t.Fatalf("expected wrapped intent update failure, got %v", err)
	}
	state.mu.Lock()
	if state.commits != 0 || state.rollbacks != 1 {
		t.Fatalf("completion and intent update must be atomic: commits=%d rollbacks=%d", state.commits, state.rollbacks)
	}
	state.failUpdate = false
	state.mu.Unlock()

	if err := complete(); err != nil {
		t.Fatalf("pending intent retry failed: %v", err)
	}
	// A repeated reconciliation uses the same intent identity. The production
	// INSERT has WHERE NOT EXISTS, so this call must remain safe and successful.
	if err := complete(); err != nil {
		t.Fatalf("idempotent completion retry failed: %v", err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.commits != 2 || state.rollbacks != 1 {
		t.Fatalf("successful retries must commit without another rollback: commits=%d rollbacks=%d", state.commits, state.rollbacks)
	}
}
