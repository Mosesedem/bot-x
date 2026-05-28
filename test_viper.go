package main

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Secret string `mapstructure:"MY_SECRET"`
	DB     string `mapstructure:"MY_DB"`
}

func main() {
	os.Setenv("MY_SECRET", "super_secret")
	os.Setenv("MY_DB", "postgres")

	viper.AutomaticEnv()
	viper.SetDefault("MY_DB", "default_db")

	var cfg Config
	viper.Unmarshal(&cfg)

	fmt.Printf("Secret: '%s'\n", cfg.Secret)
	fmt.Printf("DB: '%s'\n", cfg.DB)
}
