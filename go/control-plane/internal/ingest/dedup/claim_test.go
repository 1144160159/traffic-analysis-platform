package dedup

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestClaimBatchScopesIdentityAndExcludesConcurrentPublisher(t *testing.T) {
	d, err := NewDeduplicator(DefaultDedupConfig(), nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	var wg sync.WaitGroup
	statuses := make(chan ClaimStatus, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claims, claimErr := d.ClaimBatch(ctx, "tenant-a", "probe-a", []string{"same-event"})
			if claimErr != nil {
				t.Errorf("claim: %v", claimErr)
				return
			}
			statuses <- claims[0].Status
		}()
	}
	wg.Wait()
	close(statuses)

	acquired, inFlight := 0, 0
	for status := range statuses {
		switch status {
		case ClaimAcquired:
			acquired++
		case ClaimInFlight:
			inFlight++
		}
	}
	if acquired != 1 || inFlight != 1 {
		t.Fatalf("acquired=%d in_flight=%d", acquired, inFlight)
	}

	otherTenant, err := d.ClaimBatch(ctx, "tenant-b", "probe-a", []string{"same-event"})
	if err != nil || otherTenant[0].Status != ClaimAcquired {
		t.Fatalf("cross-tenant claim=%+v err=%v", otherTenant, err)
	}
}

func TestCommitMakesDuplicateOnlyAfterBrokerBarrier(t *testing.T) {
	d, _ := NewDeduplicator(DefaultDedupConfig(), nil, zap.NewNop())
	ctx := context.Background()
	claims, err := d.ClaimBatch(ctx, "tenant-a", "probe-a", []string{"event-1"})
	if err != nil || claims[0].Status != ClaimAcquired {
		t.Fatalf("initial claim=%+v err=%v", claims, err)
	}
	d.ReleaseBatch(ctx, claims)
	retry, _ := d.ClaimBatch(ctx, "tenant-a", "probe-a", []string{"event-1"})
	if retry[0].Status != ClaimAcquired {
		t.Fatalf("released claim status=%v want acquired", retry[0].Status)
	}
	if err := d.CommitBatch(ctx, retry); err != nil {
		t.Fatal(err)
	}
	duplicate, _ := d.ClaimBatch(ctx, "tenant-a", "probe-a", []string{"event-1"})
	if duplicate[0].Status != ClaimDuplicateCommitted {
		t.Fatalf("committed claim status=%v want duplicate", duplicate[0].Status)
	}
}
