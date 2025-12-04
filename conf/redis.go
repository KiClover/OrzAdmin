package conf

type RedisConf struct {
	Host     string `json:",env=REDIS_HOST"`
	Port     int    `json:",env=REDIS_PORT"`
	User     string `json:",optional,env=REDIS_USER"`
	Password string `json:",optional,env=REDIS_PASSWORD"`
	DB       int    `json:",optional,env=REDIS_DB"`
}
