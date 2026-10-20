package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWithoutFile(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "missing.yml")); err != nil {
		t.Fatal(err)
	}
	c := Get()
	if c.Server.Port != 3000 || c.Forecasts.Amount != 20 || c.ThemeParks.ParkID == "" || c.Forecasts.Interval.Max != 45 {
		t.Errorf("unexpected defaults %+v", c)
	}
}

func TestLoadFileAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	err := os.WriteFile(path, []byte(`
server:
  port: 8080
forecasts:
  priorityExponent: 0.8
  interval:
    min: 10
    max: 20
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PREVISIX_DATABASE_URI", "mongodb://db:27017/custom")
	t.Setenv("PREVISIX_FORECASTS_INTERVAL_MAX", "30")

	if err = Load(path); err != nil {
		t.Fatal(err)
	}
	c := Get()
	if c.Server.Port != 8080 || c.Forecasts.PriorityExponent != 0.8 || c.Forecasts.Interval.Min != 10 {
		t.Errorf("file values not loaded: %+v", c)
	}
	if c.Database.URI != "mongodb://db:27017/custom" || c.Forecasts.Interval.Max != 30 {
		t.Errorf("environment not loaded: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("asterix:\n  endpoint: https://example.com/graphql\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err == nil {
		t.Error("an endpoint without query must be rejected")
	}

	t.Setenv("PREVISIX_FORECASTS_INTERVAL_MIN", "60")
	if err := Load(""); err == nil {
		t.Error("an interval with min > max must be rejected")
	}
}
