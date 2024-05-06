package config

import (
	"github.com/spf13/viper"
)

func Load(configPath string) error {
	// TODO: load secrets from .env
	// err := godotenv.Load()
	// if err != nil {
	// 	 return err
	// }

	viper.SetConfigFile(configPath)

	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	err = viper.Unmarshal(&C)

	return err
}
