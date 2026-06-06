package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

type IPValidator struct {
	allowPrivate        bool
	allowLoopback       bool
	allowIPv4MappedIPv6 bool
}

func NewIPValidator() *IPValidator {
	return &IPValidator{
		allowPrivate:        true,
		allowLoopback:       true,
		allowIPv4MappedIPv6: true,
	}
}

func (v *IPValidator) WithAllowPrivate(allow bool) *IPValidator {
	v.allowPrivate = allow
	return v
}

func (v *IPValidator) WithAllowLoopback(allow bool) *IPValidator {
	v.allowLoopback = allow
	return v
}

func (v *IPValidator) WithAllowIPv4MappedIPv6(allow bool) *IPValidator {
	v.allowIPv4MappedIPv6 = allow
	return v
}

func (v *IPValidator) IsValidIP(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	if !v.allowPrivate && parsedIP.IsPrivate() {
		return false
	}

	return true
}

func (v *IPValidator) IsValidIPv4(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	if parsedIP.To4() == nil {
		return false
	}

	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	if !v.allowPrivate && parsedIP.IsPrivate() {
		return false
	}

	return true
}

func (v *IPValidator) IsValidIPv6(ip string) bool {
	if ip == "" {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	if parsedIP.To4() != nil {

		if v.allowIPv4MappedIPv6 && isIPv4MappedIPv6(ip) {

			return true
		}

		return false
	}

	if !v.allowLoopback && parsedIP.IsLoopback() {
		return false
	}

	return true
}

func isIPv4MappedIPv6(ip string) bool {

	if !strings.Contains(ip, ":") {
		return false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	ipLower := strings.ToLower(ip)
	if strings.HasPrefix(ipLower, "::ffff:") {
		return true
	}

	if len(parsedIP) == net.IPv6len {

		for i := 0; i < 10; i++ {
			if parsedIP[i] != 0 {
				return false
			}
		}
		if parsedIP[10] == 0xff && parsedIP[11] == 0xff {
			return true
		}
	}

	return false
}

func (v *IPValidator) IsValidCIDR(cidr string) bool {
	if cidr == "" {
		return false
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	if ipNet.IP.String() != ipNet.IP.Mask(ipNet.Mask).String() {

	}

	return true
}

func (v *IPValidator) IsValidIPOrCIDR(input string) bool {
	if v.IsValidIP(input) {
		return true
	}
	return v.IsValidCIDR(input)
}

func NormalizeIP(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	if isIPv4MappedIPv6(ip) {
		ipv4 := parsedIP.To4()
		if ipv4 != nil {
			return ipv4.String()
		}
	}

	return parsedIP.String()
}

func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsPrivate()
}

func IsLoopbackIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsLoopback()
}

func IsMulticastIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsMulticast()
}

func IsLinkLocalIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return parsedIP.IsLinkLocalUnicast()
}

func GetIPVersion(ip string) int {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return 0
	}

	if isIPv4MappedIPv6(ip) {
		return 4
	}

	if parsedIP.To4() != nil {
		return 4
	}
	return 6
}

func GetIPType(ip string) IPType {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return IPTypeInvalid
	}

	result := IPType{
		Valid:            true,
		Version:          GetIPVersion(ip),
		IsPrivate:        parsedIP.IsPrivate(),
		IsLoopback:       parsedIP.IsLoopback(),
		IsMulticast:      parsedIP.IsMulticast(),
		IsLinkLocal:      parsedIP.IsLinkLocalUnicast(),
		IsIPv4MappedIPv6: isIPv4MappedIPv6(ip),
	}

	return result
}

type IPType struct {
	Valid            bool
	Version          int
	IsPrivate        bool
	IsLoopback       bool
	IsMulticast      bool
	IsLinkLocal      bool
	IsIPv4MappedIPv6 bool
}

var IPTypeInvalid = IPType{Valid: false}

func SanitizeIP(ip string) (string, bool) {

	ip = strings.TrimSpace(ip)

	if containsDangerousChars(ip) {
		return "", false
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "", false
	}

	return NormalizeIP(ip), true
}

var dangerousCharsRegex = regexp.MustCompile(`[;'"\\<>(){}]|\-\-|/\*|\*/`)

func containsDangerousChars(s string) bool {
	return dangerousCharsRegex.MatchString(s)
}

func ValidateIPList(ips []string) ([]string, []string) {
	validator := NewIPValidator()
	valid := make([]string, 0, len(ips))
	invalid := make([]string, 0)

	for _, ip := range ips {
		sanitized, ok := SanitizeIP(ip)
		if ok && validator.IsValidIP(sanitized) {
			valid = append(valid, sanitized)
		} else {
			invalid = append(invalid, ip)
		}
	}

	return valid, invalid
}

type IPRange struct {
	Start net.IP
	End   net.IP
}

func ParseIPRange(input string) (*IPRange, error) {

	if strings.Contains(input, "/") {
		_, ipNet, err := net.ParseCIDR(input)
		if err != nil {
			return nil, err
		}

		start := ipNet.IP
		end := make(net.IP, len(start))
		copy(end, start)

		for i := range ipNet.Mask {
			end[i] |= ^ipNet.Mask[i]
		}

		return &IPRange{Start: start, End: end}, nil
	}

	if strings.Contains(input, "-") {
		parts := strings.SplitN(input, "-", 2)
		if len(parts) != 2 {
			return nil, &net.ParseError{Type: "IP range", Text: input}
		}

		start := net.ParseIP(strings.TrimSpace(parts[0]))
		end := net.ParseIP(strings.TrimSpace(parts[1]))

		if start == nil || end == nil {
			return nil, &net.ParseError{Type: "IP range", Text: input}
		}

		return &IPRange{Start: start, End: end}, nil
	}

	ip := net.ParseIP(input)
	if ip == nil {
		return nil, &net.ParseError{Type: "IP address", Text: input}
	}

	return &IPRange{Start: ip, End: ip}, nil
}

func (r *IPRange) Contains(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	return bytesCompare(parsedIP, r.Start) >= 0 && bytesCompare(parsedIP, r.End) <= 0
}

func bytesCompare(a, b net.IP) int {

	if len(a) != len(b) {
		if a4 := a.To4(); a4 != nil {
			a = a4
		}
		if b4 := b.To4(); b4 != nil {
			b = b4
		}
	}

	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}

	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

type IPPool struct {
	ranges []*IPRange
}

func NewIPPool(cidrs []string) (*IPPool, error) {
	pool := &IPPool{
		ranges: make([]*IPRange, 0, len(cidrs)),
	}

	for _, cidr := range cidrs {
		r, err := ParseIPRange(cidr)
		if err != nil {
			return nil, err
		}
		pool.ranges = append(pool.ranges, r)
	}

	return pool, nil
}

func (p *IPPool) Contains(ip string) bool {
	for _, r := range p.ranges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *IPPool) Add(cidr string) error {
	r, err := ParseIPRange(cidr)
	if err != nil {
		return err
	}
	p.ranges = append(p.ranges, r)
	return nil
}

var (
	PrivateIPv4Ranges = []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	LoopbackRanges = []string{
		"127.0.0.0/8",
		"::1/128",
	}

	LinkLocalRanges = []string{
		"169.254.0.0/16",
		"fe80::/10",
	}

	MulticastRanges = []string{
		"224.0.0.0/4",
		"ff00::/8",
	}
)

func IsInPrivateRange(ip string) bool {
	pool, _ := NewIPPool(PrivateIPv4Ranges)
	return pool.Contains(ip)
}

func IsInLoopbackRange(ip string) bool {
	pool, _ := NewIPPool(LoopbackRanges)
	return pool.Contains(ip)
}

func ExpandIPv6(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	if parsedIP.To4() != nil && !isIPv4MappedIPv6(ip) {
		return ip
	}

	ipv6 := parsedIP.To16()
	if ipv6 == nil {
		return ip
	}

	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		ipv6[0], ipv6[1], ipv6[2], ipv6[3], ipv6[4], ipv6[5], ipv6[6], ipv6[7],
		ipv6[8], ipv6[9], ipv6[10], ipv6[11], ipv6[12], ipv6[13], ipv6[14], ipv6[15])
}

func CompressIPv6(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ip
	}

	if parsedIP.To4() != nil && !isIPv4MappedIPv6(ip) {
		return ip
	}

	return parsedIP.String()
}
