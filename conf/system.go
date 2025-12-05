package conf

type SystemConf struct {
	Network string `json:",default=tcp,env=NETWORK"`
	Addr    string `json:",default=0.0.0.0,env=ADDR"`
	Port    int    `json:",default=8080,env=PORT"`
}
