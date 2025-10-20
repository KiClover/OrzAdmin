package conf

type LoggerConfig struct {
	Level      string `json:",optional,env=LOGGER_LEVEL,default=info"`
	Formatter  string `json:",optional,env=LOGGER_FORMATTER,default=text"`
	OutputMode string `json:",optional,env=LOGGER_OUTPUT_MODE,default=all"`

	// File
	FilePath string `json:",optional,env=LOGGER_FILEPATH,default=./orzFrp.log"`
}
