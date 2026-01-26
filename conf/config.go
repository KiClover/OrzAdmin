package conf

type Config struct {
	DatabaseConf       DatabaseConf
	RedisConf          RedisConf
	SystemConf         SystemConf
	DiscoveryConf      DiscoveryConf
	EtcdConf           EtcdConf
	NacosConf          NacosConf
	AuthConf           AuthConf
	CasbinDatabaseConf DatabaseConf
	TokenDatabaseConf  DatabaseConf
}
