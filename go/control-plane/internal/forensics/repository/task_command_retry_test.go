package repository

import (
	"testing"
	"time"
)

func TestApplyTaskMutationRetryPreservesRequestAndResetsResults(t *testing.T) {
	completedAt := time.Now().UTC()
	task := &Task{
		Status: TaskStatusFailed, Progress: 81, ParamsJSON: []byte(`{"purpose":"incident-response"}`),
		ResultFileKey: "results/old.pcap", ResultSHA256: "deadbeef", ResultPackets: 12,
		ResultBytes: 34, FilesScanned: 2, ErrorMessage: "source temporarily unavailable", CompletedAt: &completedAt,
	}
	paramsBefore := string(task.ParamsJSON)
	if err := applyTaskMutation(task, taskMutation{Operation: "retry", Status: TaskStatusQueued}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if task.Status != TaskStatusQueued || task.Progress != 0 || task.ErrorMessage != "" || task.CompletedAt != nil ||
		task.ResultFileKey != "" || task.ResultSHA256 != "" || task.ResultPackets != 0 || task.ResultBytes != 0 || task.FilesScanned != 0 {
		t.Fatalf("retry did not reset execution result: %+v", task)
	}
	if string(task.ParamsJSON) != paramsBefore {
		t.Fatalf("retry changed immutable params: before=%s after=%s", paramsBefore, task.ParamsJSON)
	}
}

func TestApplyTaskMutationRetryRejectsActiveTask(t *testing.T) {
	for _, status := range []string{TaskStatusQueued, TaskStatusProcessing, TaskStatusCompleted} {
		t.Run(status, func(t *testing.T) {
			if err := applyTaskMutation(&Task{Status: status}, taskMutation{Operation: "retry"}, time.Now().UTC()); err == nil {
				t.Fatalf("retry unexpectedly accepted status %s", status)
			}
		})
	}
}
