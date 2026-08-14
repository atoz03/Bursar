package main

import "testing"

func envLookup(pairs map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := pairs[name]
		return v, ok
	}
}

func TestApplyEnvOverridesReplacesNonEmptyValues(t *testing.T) {
	cfg := Config{
		ListenAddr:   "0.0.0.0:8080",
		DatabaseDSN:  "postgres://from-yaml/db",
		AdminToken:   "yaml-admin",
		CookieSecure: false,
	}
	err := cfg.applyEnvOverrides(envLookup(map[string]string{
		"GPUOPS_DATABASE_DSN":  "postgres://from-env/db",
		"GPUOPS_ADMIN_TOKEN":   "env-admin",
		"GPUOPS_COOKIE_SECURE": "true",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseDSN != "postgres://from-env/db" {
		t.Fatalf("database_dsn not overridden: %s", cfg.DatabaseDSN)
	}
	if cfg.AdminToken != "env-admin" {
		t.Fatalf("admin_token not overridden: %s", cfg.AdminToken)
	}
	if !cfg.CookieSecure {
		t.Fatal("cookie_secure not overridden")
	}
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("listen_addr should keep the YAML value: %s", cfg.ListenAddr)
	}
}

func TestApplyEnvOverridesIgnoresEmptyAndUnset(t *testing.T) {
	cfg := Config{ListenAddr: "0.0.0.0:8080", AdminToken: "yaml-admin", DryRun: true}
	err := cfg.applyEnvOverrides(envLookup(map[string]string{
		"GPUOPS_ADMIN_TOKEN": "   ",
		"GPUOPS_DRY_RUN":     "",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdminToken != "yaml-admin" {
		t.Fatalf("empty env must not clear the YAML value: %q", cfg.AdminToken)
	}
	if !cfg.DryRun {
		t.Fatal("empty env must not flip dry_run")
	}
}

func TestApplyEnvOverridesRejectsInvalidBool(t *testing.T) {
	cfg := Config{}
	if err := cfg.applyEnvOverrides(envLookup(map[string]string{"GPUOPS_DRY_RUN": "maybe"})); err == nil {
		t.Fatal("expected an error for a non-boolean value")
	}
}
