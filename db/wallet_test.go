package db

import (
	"context"
	"testing"
)

// 表客户端缺失时必须报错：运行时会把"没有数据"当作未初始化并补默认配置，
// 所以查询故障绝不能伪装成空库。
func TestHasBusinessDataRejectsUnconfiguredDatabase(t *testing.T) {
	cases := map[string]*WalletServiceDB{
		"nil store":   nil,
		"empty store": {},
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			present, err := store.HasBusinessData(context.Background())
			if err == nil {
				t.Fatal("未配置的表客户端必须返回错误，不能被当成没有数据")
			}
			if present {
				t.Fatal("未配置的表客户端不能报告已有数据")
			}
		})
	}
}
