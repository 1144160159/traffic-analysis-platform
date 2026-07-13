package audit

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestWriteToBackupDoesNotDeadlockDuringRotationCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit-service_"+time.Now().Format("2006-01-02")+".jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("open backup file: %v", err)
	}

	logger := &Logger{
		logger:        zap.NewNop(),
		serviceName:   "audit-service",
		backupDir:     dir,
		backupEnabled: true,
		backupFile:    file,
		backupWriter:  bufio.NewWriter(file),
	}
	t.Cleanup(func() { logger.closeBackup() })

	done := make(chan error, 1)
	go func() {
		done <- logger.writeToBackup(&AuditEvent{
			EventID:   "audit-1",
			EventType: EventTypeDataIngested,
			Timestamp: time.Now(),
			Action:    "test",
			Result:    ResultSuccess,
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writeToBackup() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("writeToBackup() deadlocked while checking backup rotation")
	}
}
