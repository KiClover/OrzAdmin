package conf

// LoggerConf stores unified logging configurations.
type LoggerConf struct {
	Level      string `mapstructure:"level"`        // debug|info|warn|error，默认 info
	OutputMode string `mapstructure:"output_mode"`  // console|file|all，默认 console
	Formatter  string `mapstructure:"formatter"`    // module_text|text|json，默认 module_text
	FilePath   string `mapstructure:"file_path"`    // file/all 模式必填；基础路径，实际文件名追加日期
	MaxAgeDays int    `mapstructure:"max_age_days"` // 保留天数，默认 30
	Compress   bool   `mapstructure:"compress"`     // 轮转后是否 gzip，默认 false
}
