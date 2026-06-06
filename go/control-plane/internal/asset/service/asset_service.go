package service

import (
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"go.uber.org/zap"
)

// AssetService provides MAC→IP mapping, device discovery, and asset inventory.
type AssetService struct {
	cfg    *config.Config
	repo   *repository.AssetRepository
	logger *zap.Logger
}

func New(cfg *config.Config, repo *repository.AssetRepository, logger *zap.Logger) *AssetService {
	return &AssetService{cfg: cfg, repo: repo, logger: logger}
}

// LookupVendor returns vendor name from OUI prefix.
func LookupVendor(mac string) string {
	if len(mac) < 8 { return "Unknown" }
	oui := mac[:8]
	vendors := map[string]string{
		"00:1a:c5": "Cisco Systems", "00:1b:21": "Intel Corporate",
		"00:0c:29": "VMware, Inc.", "08:00:27": "Oracle VirtualBox",
		"18:c0:09": "Broadcom Limited", "b8:27:eb": "Raspberry Pi Foundation",
		"dc:a6:32": "Raspberry Pi Trading", "00:50:56": "VMware ESX",
		"00:1b:63": "Apple, Inc.", "3c:15:c2": "Apple, Inc.",
		"00:1e:67": "Intel Corporate", "f0:1f:af": "Dell Inc.",
	}
	if v, ok := vendors[oui]; ok { return v }
	return "Unknown"
}
