package config

import "testing"

func TestRestorationRemainsDefaultOff(t *testing.T) {
	t.Setenv("RESTORATION_ENABLED", "false")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Restoration.Enabled {
		t.Fatal("restoration unexpectedly enabled by default")
	}
}

func TestEnabledRestorationRejectsUnboundedLimits(t *testing.T) {
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	config.S3.AccessKey = "test-access"
	config.S3.SecretKey = "test-secret"
	config.Restoration.Enabled = true
	config.Restoration.MaxSourceBytes = 0
	err = config.Validate()
	if err == nil {
		t.Fatal("Validate() accepted enabled restoration without bounded limits")
	}
	configError, ok := err.(*ConfigError)
	if !ok || configError.Field != "Restoration" {
		t.Fatalf("Validate() error = %v, want Restoration config error", err)
	}
}
