package validation

import "testing"

func TestIsValidMAC(t *testing.T) {
	tests := []struct {
		mac   string
		valid bool
	}{
		{"aa:bb:cc:dd:ee:ff", true},
		{"AA:BB:CC:DD:EE:FF", true},
		{"00:00:00:00:00:00", true},
		{"ff:ff:ff:ff:ff:ff", true},
		{"", false},
		{"aa:bb:cc:dd:ee", false},     // 5 组
		{"aa:bb:cc:dd:ee:ff:gg", false}, // 7 组
		{"aa-bb-cc-dd-ee-ff", false},  // 连字符
		{"gg:bb:cc:dd:ee:ff", false},  // 非十六进制
		{"aabb.ccdd.eeff", false},     // 错误分隔符
		{"not a mac", false},
	}
	for _, tt := range tests {
		result := IsValidMAC(tt.mac)
		if result != tt.valid {
			t.Errorf("IsValidMAC(%q) = %v, want %v", tt.mac, result, tt.valid)
		}
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		input  string
		output string
		ok     bool
	}{
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff", true},
		{"  aa:bb:cc:dd:ee:ff  ", "aa:bb:cc:dd:ee:ff", true},
		{"invalid", "", false},
	}
	for _, tt := range tests {
		result, err := NormalizeMAC(tt.input)
		if tt.ok && err != nil {
			t.Errorf("NormalizeMAC(%q) unexpected error: %v", tt.input, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("NormalizeMAC(%q) expected error, got %q", tt.input, result)
		}
		if tt.ok && result != tt.output {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", tt.input, result, tt.output)
		}
	}
}

func TestIsValidHostname(t *testing.T) {
	tests := []struct {
		hostname string
		valid    bool
	}{
		{"web-server", true},
		{"db01.internal.local", true},
		{"example.com", true},
		{"a", true},
		{"", false},
		{"-invalid", false},
		{"invalid-", false},
		{"host name", false},    // 空格
		{"host\nname", false},   // 换行
		{"", false},
	}
	for _, tt := range tests {
		result := IsValidHostname(tt.hostname)
		if result != tt.valid {
			t.Errorf("IsValidHostname(%q) = %v, want %v", tt.hostname, result, tt.valid)
		}
	}
}

func TestOUIFromMAC(t *testing.T) {
	tests := []struct {
		mac string
		oui string
	}{
		{"aa:bb:cc:dd:ee:ff", "aa:bb:cc"},
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc"},
		{"invalid", ""},
	}
	for _, tt := range tests {
		result := OUIFromMAC(tt.mac)
		if result != tt.oui {
			t.Errorf("OUIFromMAC(%q) = %q, want %q", tt.mac, result, tt.oui)
		}
	}
}

func TestIsValidVendor(t *testing.T) {
	tests := []struct {
		vendor string
		valid  bool
	}{
		{"Cisco", true},
		{"Dell Inc.", true},
		{"Huawei Technologies", true},
		{"Intel-Corp", true},
		{"", false},
	}
	for _, tt := range tests {
		result := IsValidVendor(tt.vendor)
		if result != tt.valid {
			t.Errorf("IsValidVendor(%q) = %v, want %v", tt.vendor, result, tt.valid)
		}
	}
}
