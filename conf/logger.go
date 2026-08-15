package conf

type LoggerConf struct {
	Level string `json:",default=info,env=LOG_LEVEL"`
}
