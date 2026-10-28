package conf

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Deprecated: -env 仅为兼容既有启动脚本（run-all.ps1 / README）保留，值被忽略。
// 配置始终按 默认值 -> YAML(-f) -> 环境变量 合并。
var configEnvBool = flag.Bool("env", true, "deprecated: config always merges defaults, yaml file and env vars")
var configFile = flag.String("f", "etc/cfg.yaml", "config file path")

func NewLoadConf() *Config {
	flag.Parse()
	_ = *configEnvBool // 兼容保留，不再参与分支
	return loadConfig(*configFile)
}

func loadConfig(configFile string) *Config {
	v := viper.New()

	// 1. 默认值（注册键，保证 AutomaticEnv+Unmarshal 能发现嵌套 logger 键）
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.output_mode", "console")
	v.SetDefault("logger.formatter", "module_text")
	v.SetDefault("logger.max_age_days", 30)
	v.SetDefault("logger.compress", false)

	// 显式环境变量绑定（不依赖 Viper 实验特性）
	mustBindEnv(v, "logger.level", "LOG_LEVEL")
	mustBindEnv(v, "logger.output_mode", "LOG_OUTPUT_MODE")
	mustBindEnv(v, "logger.formatter", "LOG_FORMATTER")
	mustBindEnv(v, "logger.file_path", "LOG_FILE_PATH")
	mustBindEnv(v, "logger.max_age_days", "LOG_MAX_AGE_DAYS")
	mustBindEnv(v, "logger.compress", "LOG_COMPRESS")

	// 2. YAML（文件不存在仅警告，继续用默认值与环境变量）
	if _, statErr := os.Stat(configFile); statErr == nil {
		v.SetConfigFile(configFile)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			panic(fmt.Errorf("failed to read config file %s : %v", configFile, err))
		}
		logrus.Infof("load config from file %s", configFile)
	} else if os.IsNotExist(statErr) {
		logrus.Warnf("config file %s not found, using defaults and environment variables", configFile)
	} else {
		panic(fmt.Errorf("failed to stat config file %s : %v", configFile, statErr))
	}

	// 3. 环境变量覆盖（保持既有 ORZFRP_ 前缀契约）
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("orzfrp")
	v.AutomaticEnv()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Errorf("failed to unmarshal config: %v", err))
	}
	return cfg
}

func mustBindEnv(v *viper.Viper, key, env string) {
	if err := v.BindEnv(key, env); err != nil {
		panic(fmt.Errorf("failed to bind env %s to %s: %v", env, key, err))
	}
}
