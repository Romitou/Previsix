package config

type Config struct {
	Database struct {
		URI string
		// Name of the database. When empty, the database from the URI is used, or "previsix".
		Name string
	}
	Server struct {
		Host string
		Port int
		// Gin mode: "release", "debug" or "test".
		Mode string
	}
	// Location of the park, used for weather forecasts.
	Park struct {
		Latitude  float64
		Longitude float64
	}
	// Official Parc Astérix GraphQL API. Optional: when the endpoint is empty, the
	// ThemeParks.wiki schedule is used instead.
	Asterix struct {
		Endpoint string
		Queries  struct {
			Calendar string
		}
	}
	ThemeParks struct {
		Endpoint string
		ParkID   string
	}
	Sources struct {
		WeatherEndpoint  string
		HolidaysEndpoint string
	}
	Collector struct {
		// Interval in minutes between two wait times snapshots.
		Interval int
	}
	Forecasts struct {
		PriorityExponent float64
		Amount           int
		Concurrent       int
		Interval         struct {
			Min int
			Max int
		}
		// Minutes after which a pending forecast is considered lost and can be queued again.
		PendingTimeout int
	}
	Model struct {
		// Minimum amount of observed days required before fitting the model on real data.
		MinTrainingDays int
		// Strength of the pull towards the prior coefficients (ridge regularization).
		Regularization float64
	}
}

var C *Config

func Get() *Config {
	return C
}
