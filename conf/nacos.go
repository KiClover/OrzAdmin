package conf

type NacosConf struct {
	Addresses   []string `json:"addresses" yaml:"addresses" mapstructure:"addresses"`
	NamespaceId string   `json:"namespaceId" yaml:"namespaceId" mapstructure:"namespaceId"`
	Username    string   `json:"username" yaml:"username" mapstructure:"username"`
	Password    string   `json:"password" yaml:"password" mapstructure:"password"`
	Group       string   `json:"group" yaml:"group" mapstructure:"group"`
	Cluster     string   `json:"cluster" yaml:"cluster" mapstructure:"cluster"`
	TimeoutMs   uint64   `json:"timeoutMs" yaml:"timeoutMs" mapstructure:"timeoutMs"`
}
