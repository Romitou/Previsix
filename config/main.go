package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

func setDefaults(v *viper.Viper) {
	v.SetDefault("database.uri", "mongodb://127.0.0.1:27017/previsix")
	v.SetDefault("database.name", "")

	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 3000)
	v.SetDefault("server.mode", "release")

	v.SetDefault("park.latitude", 49.1341839)
	v.SetDefault("park.longitude", 2.5712301)

	v.SetDefault("asterix.endpoint", "")
	v.SetDefault("asterix.queries.calendar", "")

	v.SetDefault("themeparks.endpoint", "https://api.themeparks.wiki/v1")
	v.SetDefault("themeparks.parkid", "9e938687-fd99-46f3-986a-1878210378f8")

	v.SetDefault("sources.weatherendpoint", "https://api.open-meteo.com/v1/forecast")
	v.SetDefault("sources.holidaysendpoint", "https://data.education.gouv.fr/api/explore/v2.1/catalog/datasets/fr-en-calendrier-scolaire/records")

	v.SetDefault("collector.interval", 5)

	v.SetDefault("forecasts.priorityexponent", 0.6)
	v.SetDefault("forecasts.amount", 20)
	v.SetDefault("forecasts.concurrent", 2)
	v.SetDefault("forecasts.interval.min", 30)
	v.SetDefault("forecasts.interval.max", 45)
	v.SetDefault("forecasts.pendingtimeout", 15)

	v.SetDefault("model.mintrainingdays", 14)
	v.SetDefault("model.regularization", 5.0)
}

// Load reads the configuration file (optional when missing) and environment
// variables. Every key can be overridden with PREVISIX_<SECTION>_<KEY>, for
// instance PREVISIX_DATABASE_URI.
func Load(configPath string) error {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("previsix")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if configPath != "" {
		v.SetConfigFile(configPath)
		err := v.ReadInConfig()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	C = &c
	return nil
}

func (c *Config) Validate() error {
	var errs []error
	if c.Database.URI == "" {
		errs = append(errs, errors.New("database.uri is empty"))
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server.port %d is invalid", c.Server.Port))
	}
	if (c.Asterix.Endpoint == "") != (c.Asterix.Queries.Calendar == "") {
		errs = append(errs, errors.New("asterix.endpoint and asterix.queries.calendar must be set together"))
	}
	if c.Asterix.Endpoint == "" && c.ThemeParks.ParkID == "" {
		errs = append(errs, errors.New("a calendar source is required: set asterix.endpoint or themeparks.parkId"))
	}
	if c.Collector.Interval < 1 {
		errs = append(errs, errors.New("collector.interval must be at least 1 minute"))
	}
	if c.Forecasts.Amount < 1 || c.Forecasts.Concurrent < 1 {
		errs = append(errs, errors.New("forecasts.amount and forecasts.concurrent must be at least 1"))
	}
	if c.Forecasts.Interval.Min < 1 || c.Forecasts.Interval.Max < c.Forecasts.Interval.Min {
		errs = append(errs, errors.New("forecasts.interval must satisfy 1 <= min <= max"))
	}
	if c.Forecasts.PriorityExponent < 0 {
		errs = append(errs, errors.New("forecasts.priorityExponent must be positive"))
	}
	if c.Model.Regularization <= 0 {
		errs = append(errs, errors.New("model.regularization must be strictly positive"))
	}
	return errors.Join(errs...)
}
