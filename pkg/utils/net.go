package utils

import "net"

type Addr struct {
	network string
	address string
}

func (a *Addr) Network() string {
	return a.network
}

func (a *Addr) String() string {
	return a.address
}

func NewNetAddr(network, address string) *Addr {
	return &Addr{
		network: network,
		address: address,
	}
}

// GetLocalIP 获取内网IP
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", nil
}

// GetOutboundIP 获取公网 IP 地址
func GetOutboundIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
