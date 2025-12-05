package utils

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
