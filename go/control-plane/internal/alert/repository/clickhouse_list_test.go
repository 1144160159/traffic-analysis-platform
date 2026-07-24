package repository

import "testing"

func TestDecodeAlertListPage(t *testing.T) {
	alerts, err := decodeAlertListPage(`[{"tenant_id":"default","alert_id":"AL-1","src_ip":"10.0.0.1","dst_ip":"10.0.0.2","src_port":"1234","dst_port":"443","protocol":"6","alert_type":"c2","attack_phase":"command_control","score":"0.98","severity":"SEVERITY_HIGH","first_seen":"1784747000000","last_seen":"1784747100000","count":"2","status":"ALERT_STATUS_NEW","assignee":"analyst","updated_at":"1784747100000","model_version":"v2","rule_version":"r3"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].AlertID != "AL-1" || alerts[0].DstPort != 443 || alerts[0].ModelVersion != "v2" || alerts[0].AttackPhase != "command_control" {
		t.Fatalf("decoded alerts=%+v", alerts)
	}
}
