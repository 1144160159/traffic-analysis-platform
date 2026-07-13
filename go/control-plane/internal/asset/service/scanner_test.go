package service

import "testing"

func TestParseDiscoveryTargetsCIDRHonorsMaxHosts(t *testing.T) {
	targets, err := parseDiscoveryTargets("10.10.0.0/29", 161, 3)
	if err != nil {
		t.Fatalf("parse targets: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("targets=%d, want 3", len(targets))
	}
	if targets[0].Host != "10.10.0.1" || targets[0].Port != 161 {
		t.Fatalf("first target=%+v", targets[0])
	}
}

func TestParseDiscoveryTargetsHostPortList(t *testing.T) {
	targets, err := parseDiscoveryTargets("snmp://10.0.0.8:1161,10.0.0.9", 161, 10)
	if err != nil {
		t.Fatalf("parse targets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets=%d, want 2", len(targets))
	}
	if targets[0].Host != "10.0.0.8" || targets[0].Port != 1161 {
		t.Fatalf("first target=%+v", targets[0])
	}
	if targets[1].Host != "10.0.0.9" || targets[1].Port != 161 {
		t.Fatalf("second target=%+v", targets[1])
	}
}

func TestDerivedDiscoveryMACIsStableAndLocal(t *testing.T) {
	first := derivedDiscoveryMAC("10.0.0.8")
	second := derivedDiscoveryMAC("10.0.0.8")
	if first != second {
		t.Fatalf("derived MAC should be stable: %s != %s", first, second)
	}
	if first[:5] != "02:ad" {
		t.Fatalf("derived MAC should use local-admin prefix, got %s", first)
	}
}
