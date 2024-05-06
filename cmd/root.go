package cmd

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/scheduler"
	"github.com/spf13/cobra"
	"log"
	"os"
	"strconv"
)

var configFile string

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "config.yml", "config file (default is ./config.yml)")
}

var rootCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Previsix web server",
	Run: func(cmd *cobra.Command, args []string) {
		err := config.Load(configFile)
		if err != nil {
			log.Panic("error loading config: ", err)
		}

		err = database.Connect()
		if err != nil {
			log.Panic("error connecting to database: ", err)
		}

		err = scheduler.Start()
		if err != nil {
			log.Panic("error starting scheduler: ", err)
		}

		r := gin.Default()
		// TODO: Add routes here
		err = r.Run(config.Get().Server.Host, strconv.Itoa(config.Get().Server.Port))
		if err != nil {
			log.Panic("error starting http server: ", err)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
