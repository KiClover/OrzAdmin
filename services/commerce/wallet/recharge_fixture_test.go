package wallet

import (
	"encoding/json"
	"testing"

	"github.com/KiClover/orzAdmin/kitex_gen/commerce/wallet"
	settings "github.com/KiClover/orzAdmin/kitex_gen/system/setting"
	"github.com/KiClover/orzAdmin/services/commerce/wallet/payment"
	creemprovider "github.com/KiClover/orzAdmin/services/commerce/wallet/provider/creem"
)

// 任务 6 的共享测试夹具：Provider 侧的固定标识与一份使用 TEST Profile 的 Setting 快照。
const (
	testProfileID     = "wp_test_1"
	testProductID     = "prod_test_1"
	testAPIKey        = "creem_test_key"
	testWebhookSecret = "whsec_test_secret"
	testCheckoutID    = "chk_test_1"
	testOrderID       = "ord_test_1"
	testTransactionID = "txn_test_1"
	testSuccessURL    = "https://shop.example.com/wallet/success"
)

// rechargeSnapshot 构造启用 TEST 充值模式的运行时快照。
func rechargeSnapshot() *settings.WalletRuntimeSettings {
	return &settings.WalletRuntimeSettings{
		Revision:             11,
		Enabled:              true,
		RechargeMode:         rechargeModeTest,
		WalletCurrency:       currencyCNY,
		ProviderCurrency:     currencyUSD,
		CnyPerUsdRate:        "7.200000",
		CnyPerUsdNumerator:   720,
		CnyPerUsdDenominator: 100,
		UpdatedAt:            testNow.Unix(),
		TestProfile: &settings.WalletProviderProfileRuntime{
			Environment:     providerEnvironmentTest,
			ProfileId:       testProfileID,
			ProfileRevision: 3,
			ApiKey:          testAPIKey,
			ProductId:       testProductID,
			WebhookId:       "wh_test_1",
			WebhookSecrets: []*settings.WalletWebhookSecretRuntime{
				{EndpointId: "ep_test_1", Fingerprint: "fp", Secret: testWebhookSecret},
			},
		},
	}
}

// paidTransaction 构造已支付交易。
func paidTransaction() payment.TransactionResult {
	amount := int64(1389)
	return payment.TransactionResult{
		ID:         testTransactionID,
		Mode:       "test",
		Currency:   currencyUSD,
		Type:       "payment",
		Status:     payment.TransactionStatusPaid,
		RawStatus:  "paid",
		Amount:     amount,
		AmountPaid: &amount,
		OrderID:    testOrderID,
	}
}

// refundedTransaction 构造已全额退款交易。
func refundedTransaction() payment.TransactionResult {
	result := paidTransaction()
	result.Status = payment.TransactionStatusRefunded
	result.RawStatus = "refunded"
	refunded := result.Amount
	result.RefundedAmount = &refunded
	return result
}

// webhookBody 构造 transaction.paid 事件。
func webhookBody() []byte {
	body := map[string]any{
		"id":        "evt_test_1",
		"eventType": "transaction.paid",
		"object": map[string]any{
			"id":     testTransactionID,
			"object": "transaction",
			"order": map[string]any{
				"id":          testOrderID,
				"checkout":    testCheckoutID,
				"transaction": testTransactionID,
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return encoded
}

// webhookRequest 用真实 Secret 签名。
func webhookRequest(t *testing.T, body []byte, secret string) *wallet.ProcessPaymentWebhookReq {
	t.Helper()
	return &wallet.ProcessPaymentWebhookReq{
		Provider:   wallet.PaymentProvider_PAYMENT_PROVIDER_CREEM,
		Body:       body,
		ReceivedAt: testNow.UnixMilli(),
		Headers: []*wallet.WebhookHeader{
			{Name: "creem-signature", Values: []string{creemprovider.SignWebhookPayload(secret, body)}},
		},
	}
}
