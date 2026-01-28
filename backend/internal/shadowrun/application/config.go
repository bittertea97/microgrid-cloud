package application

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Thresholds defines diff thresholds.
type Thresholds struct {
	EnergyAbs     float64 `yaml:"energy_abs"`
	EnergyPct     float64 `yaml:"energy_pct"`
	AmountAbs     float64 `yaml:"amount_abs"`
	AmountPct     float64 `yaml:"amount_pct"`
	MissingHours  int     `yaml:"missing_hours"`
	LateDataCount int     `yaml:"late_data"`
}

// Config defines shadowrun configuration.
type Config struct {
	Defaults      Thresholds            `yaml:"defaults"`
	Stations      map[string]Thresholds `yaml:"stations"`
	Schedule      ScheduleConfig        `yaml:"schedule"`
	StorageRoot   string                `yaml:"storage_root"`
	WebhookURL    string                `yaml:"webhook_url"`
	PublicBaseURL string                `yaml:"public_base_url"`
	FallbackPrice float64               `yaml:"fallback_price"`
}

// ScheduleConfig defines cron-like schedule.
type ScheduleConfig struct {
	DailyAt  string   `yaml:"daily_at"`
	Stations []string `yaml:"stations"`
}

// LoadConfig loads config from yaml or env.
func LoadConfig() (Config, error) {
	cfg := Config{
		Defaults: Thresholds{
			EnergyAbs:     1,
			EnergyPct:     0.05,
			AmountAbs:     1,
			AmountPct:     0.05,
			MissingHours:  1,
			LateDataCount: 0,
		},
		StorageRoot:   getenvDefault("SHADOWRUN_STORAGE_ROOT", filepath.FromSlash("var/reports/shadowrun")),
		WebhookURL:    os.Getenv("SHADOWRUN_WEBHOOK_URL"),
		PublicBaseURL: getenvDefault("SHADOWRUN_PUBLIC_BASE_URL", "http://localhost:8080"),
		FallbackPrice: getenvFloatDefault("PRICE_PER_KWH", 0),
	}

	if path := os.Getenv("SHADOWRUN_CONFIG"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}

	if cfg.Schedule.DailyAt == "" {
		cfg.Schedule.DailyAt = getenvDefault("SHADOWRUN_DAILY_AT", "02:00")
	}
	if len(cfg.Schedule.Stations) == 0 {
		cfg.Schedule.Stations = splitCSV(getenvDefault("SHADOWRUN_STATIONS", ""))
	}
	if cfg.WebhookURL == "" {
		cfg.WebhookURL = os.Getenv("SHADOWRUN_WEBHOOK_URL")
	}
	if cfg.StorageRoot == "" {
		return cfg, errors.New("shadowrun: storage root required")
	}
	return cfg, nil
}

// ThresholdsForStation returns thresholds for a station.
func (c Config) ThresholdsForStation(stationID string) Thresholds {
	if c.Stations != nil {
		if override, ok := c.Stations[stationID]; ok {
			return mergeThresholds(c.Defaults, override)
		}
	}
	return c.Defaults
}

func mergeThresholds(base, override Thresholds) Thresholds {
	if override.EnergyAbs != 0 {
		base.EnergyAbs = override.EnergyAbs
	}
	if override.EnergyPct != 0 {
		base.EnergyPct = override.EnergyPct
	}
	if override.AmountAbs != 0 {
		base.AmountAbs = override.AmountAbs
	}
	if override.AmountPct != 0 {
		base.AmountPct = override.AmountPct
	}
	if override.MissingHours != 0 {
		base.MissingHours = override.MissingHours
	}
	if override.LateDataCount != 0 {
		base.LateDataCount = override.LateDataCount
	}
	return base
}

func getenvDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvFloatDefault(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
