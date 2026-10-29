package trace

// SetEnabledForTest 仅供本包外的测试切换开关状态。
func SetEnabledForTest(v bool) { enabledFlag.Store(v) }
