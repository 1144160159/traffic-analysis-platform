package repository

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestScanAlertListRowPreservesTypedColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seen := time.UnixMilli(1784747100000).UTC()
	mock.ExpectQuery("SELECT typed_alert_page").WillReturnRows(sqlmock.NewRows([]string{
		"tenant_id", "alert_id", "src_ip", "dst_ip", "src_port", "dst_port",
		"protocol", "alert_type", "score", "severity", "first_seen", "last_seen",
		"count", "status", "assignee", "updated_at", "model_version", "rule_version",
		"attack_phase",
	}).AddRow(
		"default", "AL-1", "10.0.0.1", "10.0.0.2", uint32(1234), uint32(443),
		uint32(6), "c2", float32(0.98), "SEVERITY_HIGH", seen.Add(-time.Minute), seen,
		int32(2), "ALERT_STATUS_NEW", "analyst", seen, "v2", "r3", "command_control",
	))
	row := db.QueryRow("SELECT typed_alert_page")
	alert, err := scanAlertListRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if alert.AlertID != "AL-1" || alert.DstPort != 443 || alert.Protocol != 6 ||
		alert.ModelVersion != "v2" || alert.AttackPhase != "command_control" ||
		!alert.LastSeen.Equal(seen) {
		t.Fatalf("scanned alert=%+v", alert)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
