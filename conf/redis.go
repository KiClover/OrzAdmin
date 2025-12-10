package conf

type RedisConf struct {
	Host     string `json:",env=REDIS_HOST"`
	User     string `json:",optional,env=REDIS_USER"`
	Password string `json:",optional,env=REDIS_PASSWORD"`
	DB       int    `json:",optional,env=REDIS_DB"`
	Master   string `json:",optional,env=REDIS_MASTER"`
	TLS      bool   `json:",optional,env=REDIS_TLS"`
}
