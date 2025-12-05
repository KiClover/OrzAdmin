package conf

import (
	"flag"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var configEnvBool = flag.Bool("env", true, "load config from env")
var configFile = flag.String("f", "etc/cfg.yaml", "config file path")

func NewLoadConf() *Config {
	flag.Parse()

	v := viper.New()

	if *configEnvBool {
		logrus.Infof("loading config from environment variables")
		v.AutomaticEnv()
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.SetEnvPrefix("orzfrp")
	} else {
		logrus.Infof("load config from file")
		v.SetConfigFile(*configFile)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			panic(fmt.Errorf("failed to read config file %s : %v", *configFile, err))
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Errorf("failed to unmarshal config: %v", err))
	}
	return cfg
}
