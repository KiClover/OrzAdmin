package commerce

import (
	"fmt"

	systemclient "github.com/KiClover/orzAdmin/internal/client/system"
	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet/commercewalletservice"
	"github.com/KiClover/orzAdmin/services/commerce/wallet"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"
)

// NewWalletServiceClient 创建 Commerce Wallet 的服务发现客户端。
//
// 与其它服务的客户端保持一致：走服务发现、统一超时、注入 traceparent。
func NewWalletServiceClient(r *discovery.Resolver) (commercewalletservice.Client, error) {
	c, err := commercewalletservice.NewClient(
		wallet.ServiceName,
		client.WithResolver(*r),
		client.WithRPCTimeout(systemclient.DefaultRPCTimeout),
		client.WithConnectTimeout(systemclient.DefaultConnectTimeout),
		client.WithSuite(tracing.NewClientSuite()), // 注入 traceparent
	)
	if err != nil {
		return nil, fmt.Errorf("create %s client: %w", wallet.ServiceName, err)
	}
	return c, nil
}
