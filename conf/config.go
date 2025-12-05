package conf

type Config struct {
	DatabaseConf DatabaseConf
	RedisConf    RedisConf
	SystemConf   SystemConf
	EtcdConf     EtcdConf
}
