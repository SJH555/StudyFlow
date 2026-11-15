package system

// InstanceGuard 在整个进程生命周期内维持单实例声明。
type InstanceGuard interface {
	// ListenForSecondLaunch 监听二次启动请求，收到时执行回调。
	ListenForSecondLaunch(func()) error
	// StopListening 等待回调结束，但保留单实例声明。
	StopListening() error
	// Close 释放单实例声明并清理资源。
	Close() error
}
