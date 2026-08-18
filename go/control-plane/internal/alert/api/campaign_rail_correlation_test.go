package api

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestBuildCampaignRailCorrelationsSelectsUniqueJaccardWinner(t *testing.T) {
	asOf := time.Unix(1700000000, 0).UTC()
	derived := []campaignRailDerived{{TenantID: "tenant-a", CampaignID: "cep-1", EventID: "11111111-1111-4111-8111-111111111111",
		AlertIDs: []string{"alert-2", "alert-1", "alert-1"}, Position: CampaignRailSourcePosition{Topic: "campaigns.v1", Partition: 1, Offset: 9}}}
	authorities := []campaignRailAuthority{
		{TenantID: "tenant-a", CampaignID: "authority-low", AggregateEventID: "22222222-2222-4222-8222-222222222222",
			Position: CampaignRailSourcePosition{Topic: CampaignAggregateEventTopic, Partition: 0, Offset: 2},
			Members:  []campaignRailMember{{RelationID: "55555555-5555-4555-8555-555555555555", EventID: "66666666-6666-4666-8666-666666666666", AlertID: "alert-1", Revision: 1}}},
		{TenantID: "tenant-a", CampaignID: "authority-winner", AggregateEventID: "33333333-3333-4333-8333-333333333333",
			Position: CampaignRailSourcePosition{Topic: CampaignAggregateEventTopic, Partition: 0, Offset: 3},
			Members: []campaignRailMember{
				{RelationID: "88888888-8888-4888-8888-888888888888", EventID: "99999999-9999-4999-8999-999999999999", AlertID: "alert-2", Revision: 3, Position: CampaignRailSourcePosition{Topic: CampaignMembershipEventTopic, Partition: 2, Offset: 8}},
				{RelationID: "77777777-7777-4777-8777-777777777777", EventID: "44444444-4444-4444-8444-444444444444", AlertID: "alert-1", Revision: 2, Position: CampaignRailSourcePosition{Topic: CampaignMembershipEventTopic, Partition: 2, Offset: 7}},
			}},
	}

	receipts, err := buildCampaignRailCorrelations(derived, authorities, asOf, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("receipts=%d", len(receipts))
	}
	got := receipts[0]
	if got.State != "correlated" || got.AggregateCampaignID != "authority-winner" || got.Confidence != 1 || got.RelationRevision != 3 {
		t.Fatalf("unexpected winner: %+v", got)
	}
	if !reflect.DeepEqual(got.RelationIDs, []string{"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"}) ||
		!reflect.DeepEqual(got.MembershipEventIDs, []string{"44444444-4444-4444-8444-444444444444", "99999999-9999-4999-8999-999999999999"}) {
		t.Fatalf("non-canonical evidence sets: relations=%v events=%v", got.RelationIDs, got.MembershipEventIDs)
	}
	replayed, err := buildCampaignRailCorrelations(derived, authorities, asOf, 0.5)
	if err != nil || replayed[0].CorrelationID != got.CorrelationID || replayed[0].CorrelationSHA256 != got.CorrelationSHA256 {
		t.Fatalf("correlation is not reproducible: first=%+v replay=%+v err=%v", got, replayed, err)
	}
	later, err := buildCampaignRailCorrelations(derived, authorities, asOf.Add(time.Hour), 0.5)
	if err != nil || later[0].CorrelationID != got.CorrelationID ||
		later[0].CorrelationSHA256 != got.CorrelationSHA256 ||
		later[0].CorrelationKeySHA256 != got.CorrelationKeySHA256 {
		t.Fatalf("processing-time-only replay manufactured a new correlation: first=%+v later=%+v err=%v",
			got, later, err)
	}
	if later[0].AsOf.Equal(got.AsOf) {
		t.Fatal("test did not exercise a later coordinated projection snapshot")
	}
}

func TestBuildCampaignRailCorrelationsRejectsIdentityShortcutAndCrossTenant(t *testing.T) {
	derived := []campaignRailDerived{{TenantID: "tenant-a", CampaignID: "same-id", EventID: "11111111-1111-4111-8111-111111111111",
		AlertIDs: []string{"alert-a"}, Position: CampaignRailSourcePosition{Topic: "campaigns.v1"}}}
	authorities := []campaignRailAuthority{
		{TenantID: "tenant-a", CampaignID: "same-id", AggregateEventID: "22222222-2222-4222-8222-222222222222",
			Members: []campaignRailMember{{AlertID: "different-alert"}}},
		{TenantID: "tenant-b", CampaignID: "cross-tenant", AggregateEventID: "33333333-3333-4333-8333-333333333333",
			Members: []campaignRailMember{{AlertID: "alert-a"}}},
	}
	receipts, err := buildCampaignRailCorrelations(derived, authorities, time.Unix(1700000000, 0).UTC(), 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if receipts[0].State != "cep_only" || receipts[0].AggregateCampaignID != "" {
		t.Fatalf("identity or cross-tenant shortcut was accepted: %+v", receipts[0])
	}
}

func TestBuildCampaignRailCorrelationsMarksTiedWinnerConflict(t *testing.T) {
	derived := []campaignRailDerived{{TenantID: "tenant-a", CampaignID: "cep-1", EventID: "11111111-1111-4111-8111-111111111111",
		AlertIDs: []string{"alert-a", "alert-b"}, Position: CampaignRailSourcePosition{Topic: "campaigns.v1"}}}
	authorities := []campaignRailAuthority{
		{TenantID: "tenant-a", CampaignID: "authority-a", AggregateEventID: "22222222-2222-4222-8222-222222222222", Members: []campaignRailMember{{AlertID: "alert-a"}}},
		{TenantID: "tenant-a", CampaignID: "authority-b", AggregateEventID: "33333333-3333-4333-8333-333333333333", Members: []campaignRailMember{{AlertID: "alert-b"}}},
	}
	receipts, err := buildCampaignRailCorrelations(derived, authorities, time.Unix(1700000000, 0).UTC(), 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if receipts[0].State != "conflict" || receipts[0].AggregateCampaignID != "" ||
		!reflect.DeepEqual(receipts[0].PartialReasons, []string{"tied_highest_authority_candidates"}) {
		t.Fatalf("tie must remain explicit conflict: %+v", receipts[0])
	}
}

func TestValidateCampaignRailScopeRequiresTenantClosedWindowAndBudget(t *testing.T) {
	valid := CampaignRailScope{TenantID: "tenant-a", WindowFrom: time.Unix(10, 0), WindowThrough: time.Unix(20, 0), MaxCampaigns: 100}
	if _, err := normalizeCampaignRailScope(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CampaignRailScope){
		func(scope *CampaignRailScope) { scope.TenantID = "unknown" },
		func(scope *CampaignRailScope) { scope.WindowThrough = scope.WindowFrom },
		func(scope *CampaignRailScope) { scope.MaxCampaigns = 10001 },
	} {
		candidate := valid
		mutate(&candidate)
		if _, err := normalizeCampaignRailScope(candidate); err == nil {
			t.Fatalf("invalid scope accepted: %+v", candidate)
		}
	}
	invalidAsOf := valid
	invalidAsOf.AsOf = valid.WindowThrough.Add(-time.Nanosecond)
	if _, err := normalizeCampaignRailScope(invalidAsOf); err == nil {
		t.Fatal("as_of before the closed event window was accepted")
	}
}

func TestCampaignRailExactSetDiffCountsMissingAndExtraWithoutDeletingEvidence(t *testing.T) {
	expected := []campaignRailManifestItem{
		{CEPEventID: "event-a", CorrelationSHA256: "sha-a", State: "correlated"},
		{CEPEventID: "event-b", CorrelationSHA256: "sha-b", State: "cep_only"},
	}
	actual := []campaignRailManifestItem{
		{CEPEventID: "event-a", CorrelationSHA256: "sha-a", State: "correlated"},
		{CEPEventID: "event-extra", CorrelationSHA256: "sha-extra", State: "conflict"},
	}
	missing, extra := campaignRailExactSetDiff(expected, actual)
	if missing != 1 || extra != 1 {
		t.Fatalf("missing=%d extra=%d", missing, extra)
	}
}

func TestCampaignRailWorkerChecksThreeRailAdmissionBeforeSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	readinessErr := errors.New("cep consumer receipt missing")
	handler.SetCampaignRailCorrelationAdmission(func(context.Context) error { return readinessErr })
	err = handler.runCampaignRailCorrelationWindow(context.Background(), 5*time.Minute, time.Minute, 100)
	if !errors.Is(err, readinessErr) {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal("correlation worker touched SQL before admission: ", err)
	}
}

func TestCampaignRailWorkerRefusesMissingAdmission(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	if err := handler.StartCampaignRailCorrelationWorker(context.Background(), time.Minute, 5*time.Minute, time.Minute, 100); err == nil {
		t.Fatal("worker must fail closed without a three-rail readiness authority")
	}
}
