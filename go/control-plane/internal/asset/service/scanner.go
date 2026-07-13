package service

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

const (
	oidSysDescr       = ".1.3.6.1.2.1.1.1.0"
	oidSysName        = ".1.3.6.1.2.1.1.5.0"
	oidIfPhysAddress1 = ".1.3.6.1.2.1.2.2.1.6.1"

	oidLLDPRemChassisID = ".1.0.8802.1.1.2.1.4.1.1.5"
	oidLLDPRemPortDesc  = ".1.0.8802.1.1.2.1.4.1.1.8"
	oidLLDPRemSysName   = ".1.0.8802.1.1.2.1.4.1.1.9"
)

type DiscoveryScanner interface {
	Scan(ctx context.Context, req *config.ActiveDiscoveryRequest, credential *config.DiscoveryCredential) ([]config.DiscoveryObservation, error)
}

type discoveryTarget struct {
	Host string
	Port uint16
}

type SNMPDiscoveryScanner struct {
	cfg    config.DiscoveryConfig
	logger *zap.Logger
}

func NewSNMPDiscoveryScanner(cfg config.DiscoveryConfig, logger *zap.Logger) *SNMPDiscoveryScanner {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.SNMPPort == 0 {
		cfg.SNMPPort = 161
	}
	if cfg.SNMPTimeout <= 0 {
		cfg.SNMPTimeout = 3 * time.Second
	}
	if cfg.SNMPRetries < 0 {
		cfg.SNMPRetries = 0
	}
	if cfg.MaxHosts <= 0 {
		cfg.MaxHosts = 128
	}
	return &SNMPDiscoveryScanner{cfg: cfg, logger: logger}
}

func (s *SNMPDiscoveryScanner) Scan(ctx context.Context, req *config.ActiveDiscoveryRequest, credential *config.DiscoveryCredential) ([]config.DiscoveryObservation, error) {
	if req == nil {
		return nil, fmt.Errorf("discovery request is required")
	}
	mode := normalizeDiscoveryMode(req.Mode)
	if mode == "" && credential != nil {
		mode = normalizeDiscoveryMode(credential.Protocol)
	}
	if mode != config.DiscoveryModeSNMP && mode != config.DiscoveryModeSNMPLLDP {
		return nil, fmt.Errorf("scanner supports SNMP or SNMP+LLDP mode, got %q", mode)
	}

	endpoint := strings.TrimSpace(req.TargetCIDR)
	if endpoint == "" && credential != nil {
		endpoint = strings.TrimSpace(credential.Endpoint)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("target_cidr or credential endpoint is required for active scan")
	}
	community := strings.TrimSpace(s.cfg.SNMPCommunity)
	if community == "" {
		return nil, fmt.Errorf("ASSET_DISCOVERY_SNMP_COMMUNITY secret is required for active SNMP scan")
	}

	targets, err := parseDiscoveryTargets(endpoint, s.cfg.SNMPPort, s.cfg.MaxHosts)
	if err != nil {
		return nil, err
	}
	var observations []config.DiscoveryObservation
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return observations, err
		}
		observation, err := s.scanSNMPTarget(target, community, mode == config.DiscoveryModeSNMPLLDP)
		if err != nil {
			s.logger.Warn("SNMP discovery target did not return usable data",
				zap.String("target", target.Host),
				zap.Uint16("port", target.Port),
				zap.Error(err))
			continue
		}
		observations = append(observations, observation)
	}
	if len(observations) == 0 {
		return nil, fmt.Errorf("no SNMP discovery targets responded")
	}
	return observations, nil
}

func (s *SNMPDiscoveryScanner) scanSNMPTarget(target discoveryTarget, community string, includeLLDP bool) (config.DiscoveryObservation, error) {
	client := &gosnmp.GoSNMP{
		Target:    target.Host,
		Port:      target.Port,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   s.cfg.SNMPTimeout,
		Retries:   s.cfg.SNMPRetries,
		MaxOids:   gosnmp.MaxOids,
	}
	if err := client.Connect(); err != nil {
		return config.DiscoveryObservation{}, fmt.Errorf("connect SNMP: %w", err)
	}
	defer client.Conn.Close()

	packet, err := client.Get([]string{oidSysName, oidSysDescr, oidIfPhysAddress1})
	if err != nil {
		return config.DiscoveryObservation{}, fmt.Errorf("read SNMP system OIDs: %w", err)
	}

	observation := config.DiscoveryObservation{
		IPAddress:  target.Host,
		Hostname:   target.Host,
		Vendor:     "SNMP",
		OSType:     "network-device",
		SwitchPort: "snmp",
	}
	for _, variable := range packet.Variables {
		switch variable.Name {
		case oidSysName:
			if value := pduString(variable); value != "" {
				observation.Hostname = value
			}
		case oidSysDescr:
			if value := pduString(variable); value != "" {
				observation.OSType = value
				observation.Vendor = inferNetworkVendor(value)
			}
		case oidIfPhysAddress1:
			if value := pduMAC(variable); value != "" {
				observation.MACAddress = value
			}
		}
	}
	if observation.MACAddress == "" {
		observation.MACAddress = derivedDiscoveryMAC(target.Host)
	}
	if includeLLDP {
		observation.Neighbors = s.walkLLDPNeighbors(client)
	}
	return observation, nil
}

func (s *SNMPDiscoveryScanner) walkLLDPNeighbors(client *gosnmp.GoSNMP) []config.DiscoveryNeighbor {
	names := walkStringMap(s.logger, client, oidLLDPRemSysName)
	ports := walkStringMap(s.logger, client, oidLLDPRemPortDesc)
	chassis := walkMACMap(s.logger, client, oidLLDPRemChassisID)
	keys := map[string]struct{}{}
	for key := range names {
		keys[key] = struct{}{}
	}
	for key := range ports {
		keys[key] = struct{}{}
	}
	for key := range chassis {
		keys[key] = struct{}{}
	}

	neighbors := make([]config.DiscoveryNeighbor, 0, len(keys))
	for key := range keys {
		neighbor := config.DiscoveryNeighbor{
			Hostname:   names[key],
			MACAddress: chassis[key],
			Interface:  ports[key],
			Protocol:   config.DiscoveryModeLLDP,
		}
		if neighbor.MACAddress == "" {
			neighbor.MACAddress = derivedDiscoveryMAC("lldp:" + key + ":" + neighbor.Hostname + ":" + neighbor.Interface)
		}
		neighbors = append(neighbors, neighbor)
	}
	return neighbors
}

func parseDiscoveryTargets(endpoint string, fallbackPort uint16, maxHosts int) ([]discoveryTarget, error) {
	if maxHosts <= 0 {
		maxHosts = 128
	}
	var targets []discoveryTarget
	for _, rawPart := range strings.Split(endpoint, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		part = strings.TrimPrefix(strings.TrimPrefix(part, "snmp://"), "udp://")
		if strings.Contains(part, "/") {
			hosts, err := hostsFromCIDR(part, fallbackPort, maxHosts-len(targets))
			if err != nil {
				return nil, err
			}
			targets = append(targets, hosts...)
			continue
		}
		target, err := parseHostPortTarget(part, fallbackPort)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
		if len(targets) >= maxHosts {
			break
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no discovery targets parsed from %q", endpoint)
	}
	return targets, nil
}

func hostsFromCIDR(cidr string, port uint16, remaining int) ([]discoveryTarget, error) {
	if remaining <= 0 {
		return nil, nil
	}
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse discovery CIDR %q: %w", cidr, err)
	}
	ip = ip.To4()
	if ip == nil {
		return []discoveryTarget{{Host: network.IP.String(), Port: port}}, nil
	}
	var targets []discoveryTarget
	for current := append(net.IP(nil), network.IP.To4()...); network.Contains(current); incrementIP(current) {
		if len(targets) >= remaining {
			break
		}
		if isIPv4NetworkOrBroadcast(current, network) {
			continue
		}
		targets = append(targets, discoveryTarget{Host: current.String(), Port: port})
	}
	return targets, nil
}

func parseHostPortTarget(value string, fallbackPort uint16) (discoveryTarget, error) {
	host := value
	port := fallbackPort
	if strings.Contains(value, ":") {
		parsedHost, parsedPort, err := net.SplitHostPort(value)
		if err == nil {
			host = parsedHost
			p, parseErr := strconv.ParseUint(parsedPort, 10, 16)
			if parseErr != nil {
				return discoveryTarget{}, fmt.Errorf("parse discovery port %q: %w", parsedPort, parseErr)
			}
			port = uint16(p)
		} else if strings.Count(value, ":") == 1 {
			parts := strings.Split(value, ":")
			host = parts[0]
			p, parseErr := strconv.ParseUint(parts[1], 10, 16)
			if parseErr != nil {
				return discoveryTarget{}, fmt.Errorf("parse discovery port %q: %w", parts[1], parseErr)
			}
			port = uint16(p)
		}
	}
	if strings.TrimSpace(host) == "" {
		return discoveryTarget{}, fmt.Errorf("discovery target host is empty")
	}
	return discoveryTarget{Host: strings.Trim(host, "[]"), Port: port}, nil
}

func pduString(variable gosnmp.SnmpPDU) string {
	switch value := variable.Value.(type) {
	case []byte:
		return strings.TrimSpace(string(value))
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func pduMAC(variable gosnmp.SnmpPDU) string {
	if bytes, ok := variable.Value.([]byte); ok {
		return bytesToMAC(bytes)
	}
	return ""
}

func walkStringMap(logger *zap.Logger, client *gosnmp.GoSNMP, oid string) map[string]string {
	values := map[string]string{}
	err := client.Walk(oid, func(variable gosnmp.SnmpPDU) error {
		key := strings.TrimPrefix(variable.Name, oid+".")
		if value := pduString(variable); value != "" {
			values[key] = value
		}
		return nil
	})
	if err != nil && logger != nil {
		logger.Debug("LLDP string walk failed", zap.String("oid", oid), zap.Error(err))
	}
	return values
}

func walkMACMap(logger *zap.Logger, client *gosnmp.GoSNMP, oid string) map[string]string {
	values := map[string]string{}
	err := client.Walk(oid, func(variable gosnmp.SnmpPDU) error {
		key := strings.TrimPrefix(variable.Name, oid+".")
		if value := pduMAC(variable); value != "" {
			values[key] = value
		}
		return nil
	})
	if err != nil && logger != nil {
		logger.Debug("LLDP MAC walk failed", zap.String("oid", oid), zap.Error(err))
	}
	return values
}

func bytesToMAC(value []byte) string {
	if len(value) < 6 {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", value[0], value[1], value[2], value[3], value[4], value[5])
}

func derivedDiscoveryMAC(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return fmt.Sprintf("02:ad:%02x:%02x:%02x:%02x", sum[0], sum[1], sum[2], sum[3])
}

func inferNetworkVendor(sysDescr string) string {
	lower := strings.ToLower(sysDescr)
	switch {
	case strings.Contains(lower, "cisco"):
		return "Cisco Systems"
	case strings.Contains(lower, "huawei"):
		return "Huawei Technologies"
	case strings.Contains(lower, "juniper"):
		return "Juniper Networks"
	case strings.Contains(lower, "arista"):
		return "Arista Networks"
	case strings.Contains(lower, "h3c"):
		return "H3C"
	default:
		return "SNMP"
	}
}

func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func isIPv4NetworkOrBroadcast(ip net.IP, network *net.IPNet) bool {
	ones, bits := network.Mask.Size()
	if bits != 32 || ones >= 31 {
		return false
	}
	networkIP := network.IP.To4()
	if networkIP == nil {
		return false
	}
	if ip.Equal(networkIP) {
		return true
	}
	broadcast := append(net.IP(nil), networkIP...)
	for i := range broadcast {
		broadcast[i] |= ^network.Mask[i]
	}
	return ip.Equal(broadcast)
}
