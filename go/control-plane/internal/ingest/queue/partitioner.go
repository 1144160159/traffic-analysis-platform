package queue

import (
	"hash/fnv"
)

type TenantCommunityPartitioner struct {
	numPartitions int
}

func NewTenantCommunityPartitioner(numPartitions int) *TenantCommunityPartitioner {
	return &TenantCommunityPartitioner{numPartitions: numPartitions}
}

func (p *TenantCommunityPartitioner) Partition(tenantID, communityID string) int {
	key := tenantID + ":" + communityID
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()) % p.numPartitions
}
