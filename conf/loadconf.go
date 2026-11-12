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

	// trace 默认值：链路追踪是可选功能，默认关闭；开启 = trace.enabled 改 true 或 TRACE_ENABLED=true
	v.SetDefault("trace.enabled", false)
	v.SetDefault("trace.endpoint", "localhost:4317")
	v.SetDefault("trace.insecure", true)
	v.SetDefault("trace.sample_ratio", 1.0)
	v.SetDefault("trace.export_timeout", "10s")
	v.SetDefault("trace.shutdown_timeout", "5s")
	v.SetDefault("trace.environment", "")
	v.SetDefault("trace.version", "")

	// 钱包部署参数：只包含随部署变化的值（技术性限制是代码常量）。
	v.SetDefault("wallet.owner_namespace", "commerce")
	v.SetDefault("wallet.product_success_url_allowed_origins", []string{})

	// 显式环境变量绑定（不依赖 Viper 实验特性）
	mustBindEnv(v, "wallet.owner_namespace", "WALLET_OWNER_NAMESPACE")
	mustBindEnv(v, "wallet.product_success_url_allowed_origins", "WALLET_PRODUCT_SUCCESS_URL_ALLOWED_ORIGINS")
	mustBindEnv(v, "logger.level", "LOG_LEVEL")
	mustBindEnv(v, "logger.output_mode", "LOG_OUTPUT_MODE")
	mustBindEnv(v, "logger.formatter", "LOG_FORMATTER")
	mustBindEnv(v, "logger.file_path", "LOG_FILE_PATH")
	mustBindEnv(v, "logger.max_age_days", "LOG_MAX_AGE_DAYS")
	mustBindEnv(v, "logger.compress", "LOG_COMPRESS")

	mustBindEnv(v, "trace.enabled", "TRACE_ENABLED")
	mustBindEnv(v, "trace.endpoint", "TRACE_ENDPOINT")
	mustBindEnv(v, "trace.insecure", "TRACE_INSECURE")
	mustBindEnv(v, "trace.sample_ratio", "TRACE_SAMPLE_RATIO")
	mustBindEnv(v, "trace.export_timeout", "TRACE_EXPORT_TIMEOUT")
	mustBindEnv(v, "trace.shutdown_timeout", "TRACE_SHUTDOWN_TIMEOUT")
	mustBindEnv(v, "trace.environment", "TRACE_ENVIRONMENT")
	mustBindEnv(v, "trace.version", "TRACE_VERSION")

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
	if err := cfg.Wallet.Validate(); err != nil {
		panic(err)
	}
	return cfg
}

func mustBindEnv(v *viper.Viper, key, env string) {
	if err := v.BindEnv(key, env); err != nil {
		panic(fmt.Errorf("failed to bind env %s to %s: %v", env, key, err))
	}
}
