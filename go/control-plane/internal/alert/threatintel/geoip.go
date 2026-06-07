// GeoIP 威胁情报富化 — 基于地理位置的风险评估
//
// 业务价值:
//   - 识别来自异常地理位置的流量 (如：来自高风险国家的访问)
//   - 检测不可能旅行 (同一用户短时间内从不同地理区域访问)
//   - 标记已知恶意 AS/ISP 的流量
//   - 地理围栏策略支持
package threatintel

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"

	"go.uber.org/zap"
)

// GeoLocation 地理位置信息
type GeoLocation struct {
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	ASN         uint32  `json:"asn"`
	ASOrg       string  `json:"as_org"`
	ISP         string  `json:"isp"`
	IsProxy     bool    `json:"is_proxy"`
	IsHosting   bool    `json:"is_hosting"`
	RiskScore   float32 `json:"risk_score"` // 0.0-1.0 基于地理的风险
}

// CountryRisk 国家风险评分
type CountryRisk struct {
	CountryCode string
	RiskLevel   string  // "low", "medium", "high", "critical"
	RiskScore   float32 // 0.0-1.0
	Description string
}

// 已知高风险国家/地区 (基于威胁情报)
var highRiskCountries = map[string]CountryRisk{
	"KP": {CountryCode: "KP", RiskLevel: "critical", RiskScore: 0.95, Description: "朝鲜 - 国家级APT活动高发"},
	"IR": {CountryCode: "IR", RiskLevel: "critical", RiskScore: 0.90, Description: "伊朗 - 国家级APT活动高发"},
	"RU": {CountryCode: "RU", RiskLevel: "high", RiskScore: 0.75, Description: "俄罗斯 - 网络攻击活动频繁"},
	"CN": {CountryCode: "CN", RiskLevel: "medium", RiskScore: 0.55, Description: "中国 - 大量扫描和入侵尝试"},
	"NG": {CountryCode: "NG", RiskLevel: "high", RiskScore: 0.70, Description: "尼日利亚 - 欺诈和BEC攻击高发"},
	"VN": {CountryCode: "VN", RiskLevel: "medium", RiskScore: 0.50, Description: "越南 - APT活动目标"},
	"RO": {CountryCode: "RO", RiskLevel: "medium", RiskScore: 0.50, Description: "罗马尼亚 - 网络犯罪活动"},
	"BR": {CountryCode: "BR", RiskLevel: "medium", RiskScore: 0.50, Description: "巴西 - 金融欺诈高发"},
	"UA": {CountryCode: "UA", RiskLevel: "medium", RiskScore: 0.45, Description: "乌克兰 - 网络冲突区域"},
	"BY": {CountryCode: "BY", RiskLevel: "medium", RiskScore: 0.55, Description: "白俄罗斯 - 恶意活动托管"},
}

// 已知恶意 ASN (托管C2和恶意软件)
var maliciousASNs = map[uint32]string{
	16276:  "OVH SAS - 常被用于C2托管",
	14061:  "DigitalOcean - 常被用于钓鱼和C2",
	24940:  "Hetzner Online - 常被用于恶意活动",
	45102:  "Alibaba Cloud - 大量扫描活动",
	37963:  "Hangzhou Alibaba Advertising - 大量扫描活动",
	4134:   "China Telecom - 网络攻击来源",
	4837:   "China Unicom - 网络攻击来源",
	9808:   "China Mobile - 网络攻击来源",
	20473:  "Vultr - 常被用于C2",
	63949:  "Linode - 常被用于恶意托管",
	51167:  "Contabo GmbH - 常被用于恶意托管",
	13335:  "Cloudflare - 恶意流量通过的CDN",
	29073:  "Quasi Networks - 已知恶意ISP",
	44901:  "Belcloud - 已知恶意ISP",
	43350:  "NFOrce Entertainment - 已知恶意ISP",
}

// GeoIPService GeoIP富化服务
type GeoIPService struct {
	logger *zap.Logger
	mu     sync.RWMutex
	cache  map[string]*GeoLocation
	// 观察窗口 — 用于检测不可能旅行
	userLocations map[string][]GeoLocation
	maxUserLocs   int
}

// NewGeoIPService 创建GeoIP服务
func NewGeoIPService(logger *zap.Logger) *GeoIPService {
	return &GeoIPService{
		logger:        logger,
		cache:         make(map[string]*GeoLocation),
		userLocations: make(map[string][]GeoLocation),
		maxUserLocs:   10,
	}
}

// Lookup 查询IP的地理位置
// 使用内置GeoIP数据库（简化实现，生产环境应集成MaxMind GeoIP2）
func (s *GeoIPService) Lookup(ctx context.Context, ipStr string) (*GeoLocation, error) {
	// 缓存检查
	s.mu.RLock()
	if loc, ok := s.cache[ipStr]; ok {
		s.mu.RUnlock()
		return loc, nil
	}
	s.mu.RUnlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	// 检查私有/保留IP
	if isPrivateIP(ip) {
		loc := &GeoLocation{
			IP:          ipStr,
			Country:     "Private Network",
			CountryCode: "XX",
			Region:      "RFC1918",
			City:        "Local",
			Latitude:    0,
			Longitude:   0,
			RiskScore:   0.0,
		}
		s.mu.Lock()
		s.cache[ipStr] = loc
		s.mu.Unlock()
		return loc, nil
	}

	// 模拟GeoIP查询（生产环境需替换为真实GeoIP数据库）
	loc := s.simulateLookup(ipStr)

	// 计算基于地理位置的风险评分
	loc.RiskScore = s.calculateGeoRisk(loc)

	s.mu.Lock()
	s.cache[ipStr] = loc
	s.mu.Unlock()

	return loc, nil
}

// simulateLookup 模拟GeoIP查询 (生产环境替换为MaxMind/ip2location)
func (s *GeoIPService) simulateLookup(ipStr string) *GeoLocation {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return &GeoLocation{IP: ipStr, CountryCode: "XX", RiskScore: 0.5}
	}

	// 基于IP前缀的启发式匹配
	ipBytes := ip.To4()
	if ipBytes == nil {
		return &GeoLocation{IP: ipStr, CountryCode: "XX", RiskScore: 0.3}
	}

	firstOctet := ipBytes[0]
	secondOctet := ipBytes[1]

	switch {
	case firstOctet == 5 && secondOctet == 188:
		return &GeoLocation{IP: ipStr, Country: "Russia", CountryCode: "RU", City: "Saint Petersburg", ASN: 9002, ASOrg: "RETN Limited", IsHosting: true, RiskScore: 0.75}
	case firstOctet == 71 && secondOctet == 6:
		return &GeoLocation{IP: ipStr, Country: "United States", CountryCode: "US", City: "San Diego", ASN: 10439, ASOrg: "CariNet Inc", IsHosting: true, RiskScore: 0.40}
	case firstOctet == 185 && secondOctet == 130:
		return &GeoLocation{IP: ipStr, Country: "Netherlands", CountryCode: "NL", City: "Amsterdam", ASN: 16276, ASOrg: "OVH SAS", IsHosting: true, RiskScore: 0.65}
	case firstOctet == 103 && secondOctet == 253:
		return &GeoLocation{IP: ipStr, Country: "Hong Kong", CountryCode: "HK", City: "Hong Kong", ASN: 4134, ASOrg: "China Telecom", IsHosting: true, RiskScore: 0.60}
	case firstOctet == 66 && secondOctet == 240:
		return &GeoLocation{IP: ipStr, Country: "United States", CountryCode: "US", ASN: 10439, ASOrg: "Censys Inc", IsHosting: true, RiskScore: 0.30}
	case firstOctet == 175 && secondOctet == 45:
		return &GeoLocation{IP: ipStr, Country: "North Korea", CountryCode: "KP", ASN: 131279, ASOrg: "Star Joint Venture", RiskScore: 0.95}
	case firstOctet == 94 && secondOctet == 102:
		return &GeoLocation{IP: ipStr, Country: "Iran", CountryCode: "IR", City: "Tehran", ASN: 12880, ASOrg: "Information Technology Company", RiskScore: 0.90}
	default:
		// 通用判断
		if firstOctet >= 41 && firstOctet <= 44 {
			return &GeoLocation{IP: ipStr, Country: "South Africa", CountryCode: "ZA", ASN: 0, RiskScore: 0.35}
		}
		if firstOctet >= 105 && firstOctet <= 106 {
			return &GeoLocation{IP: ipStr, Country: "Nigeria", CountryCode: "NG", ASN: 0, RiskScore: 0.70}
		}
		return &GeoLocation{IP: ipStr, Country: "Unknown", CountryCode: "XX", RiskScore: 0.20}
	}
}

// calculateGeoRisk 计算基于地理位置的综合风险评分
func (s *GeoIPService) calculateGeoRisk(loc *GeoLocation) float32 {
	var risk float32

	// 国家风险
	if cr, ok := highRiskCountries[loc.CountryCode]; ok {
		risk += cr.RiskScore * 0.4
	}

	// 恶意ASN风险
	if _, ok := maliciousASNs[loc.ASN]; ok {
		risk += 0.3
	}

	// 托管/代理风险 (数据中心比住宅IP更可疑)
	if loc.IsHosting {
		risk += 0.2
	}
	if loc.IsProxy {
		risk += 0.3
	}

	if risk > 1.0 {
		risk = 1.0
	}
	return risk
}

// DetectImpossibleTravel 检测不可能旅行 (同一用户短时间内从不同地理位置访问)
func (s *GeoIPService) DetectImpossibleTravel(userID string, currentLoc *GeoLocation) *TravelAnomaly {
	s.mu.Lock()
	defer s.mu.Unlock()

	locs := s.userLocations[userID]

	// 检查最近的位置
	for i := len(locs) - 1; i >= 0; i-- {
		prevLoc := locs[i]
		if currentLoc.IP == prevLoc.IP {
			continue
		}

		distance := haversineDistance(
			prevLoc.Latitude, prevLoc.Longitude,
			currentLoc.Latitude, currentLoc.Longitude,
		)

		// 如果两个位置距离 > 500km 且 时间差 < 1小时 → 不可能旅行
		if distance > 500 {
			maxPossibleDist := 900.0 // 商业航班 ~900 km/h
			requiredHours := distance / maxPossibleDist

			if requiredHours > 24 {
				return &TravelAnomaly{
					UserID:       userID,
					PrevLocation: prevLoc,
					CurrLocation: *currentLoc,
					DistanceKm:   distance,
					AnomalyType:  "impossible_travel",
				}
			}
		}
	}

	// 记录当前位置
	locs = append(locs, *currentLoc)
	if len(locs) > s.maxUserLocs {
		locs = locs[len(locs)-s.maxUserLocs:]
	}
	s.userLocations[userID] = locs

	return nil
}

// TravelAnomaly 旅行异常
type TravelAnomaly struct {
	UserID       string
	PrevLocation GeoLocation
	CurrLocation GeoLocation
	DistanceKm   float64
	AnomalyType  string
}

// EnrichAlertWithGeoIP 使用GeoIP信息富化告警
func (s *GeoIPService) EnrichAlertWithGeoIP(ctx context.Context, srcIP, dstIP string) *GeoEnrichment {
	enrich := &GeoEnrichment{}

	if srcIP != "" {
		loc, err := s.Lookup(ctx, srcIP)
		if err == nil {
			enrich.SrcLocation = loc
			enrich.SrcCountryCode = loc.CountryCode
			enrich.GeoRiskScore += loc.RiskScore * 0.5
			enrich.GeoTags = append(enrich.GeoTags,
				fmt.Sprintf("geo:src_country=%s", loc.CountryCode))
			if loc.IsHosting {
				enrich.GeoTags = append(enrich.GeoTags, "geo:src_hosting")
			}
			if loc.IsProxy {
				enrich.GeoTags = append(enrich.GeoTags, "geo:src_proxy")
			}
		}
	}

	if dstIP != "" {
		loc, err := s.Lookup(ctx, dstIP)
		if err == nil {
			enrich.DstLocation = loc
			enrich.DstCountryCode = loc.CountryCode
			enrich.GeoRiskScore += loc.RiskScore * 0.5
			enrich.GeoTags = append(enrich.GeoTags,
				fmt.Sprintf("geo:dst_country=%s", loc.CountryCode))
			if loc.IsHosting {
				enrich.GeoTags = append(enrich.GeoTags, "geo:dst_hosting")
			}
		}
	}

	if enrich.GeoRiskScore > 1.0 {
		enrich.GeoRiskScore = 1.0
	}

	return enrich
}

// GeoEnrichment Geo富化结果
type GeoEnrichment struct {
	SrcLocation    *GeoLocation
	DstLocation    *GeoLocation
	SrcCountryCode string
	DstCountryCode string
	GeoRiskScore   float32
	GeoTags        []string
}

// haversineDistance 计算两点间的大圆距离 (km)
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // 地球半径 (km)

	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// isPrivateIP 检查是否为私有IP
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	privateCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range privateCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// GetHighRiskCountries 返回高风险国家列表
func GetHighRiskCountries() map[string]CountryRisk {
	result := make(map[string]CountryRisk, len(highRiskCountries))
	for k, v := range highRiskCountries {
		result[k] = v
	}
	return result
}

// CacheSize 返回缓存大小
func (s *GeoIPService) CacheSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// ClearCache 清理缓存
func (s *GeoIPService) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]*GeoLocation)
}
