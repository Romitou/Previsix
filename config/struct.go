package config

type Config struct {
	Database struct {
		URI string
	}
	Server struct {
		Host string
		Port int
	}
	Asterix struct {
		Endpoint string
		Queries  struct {
			Calendar string
		}
	}
	Forecasts struct {
		PriorityExponent float64
		Amount           int
		Concurrent       int
		Interval         struct {
			Min int
			Max int
		}
	}
}

var C *Config

func Get() *Config {
	return C
}
