package main

import (
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// desktopTransport 保留公开的 HTTP transport，使 Run 失败时也能将其停止。
// Wails 与我们自己的回退逻辑共用同一个已完成的 Stop 操作。
type desktopTransport struct {
	*application.HTTPTransport
	stopOnce sync.Once
	stopErr  error
}

func newDesktopTransport() *desktopTransport {
	return &desktopTransport{HTTPTransport: application.NewHTTPTransport(
		application.HTTPTransportWithLogger(application.DefaultLogger(slog.LevelInfo)),
	)}
}

func (transport *desktopTransport) Stop() error {
	transport.stopOnce.Do(func() { transport.stopErr = transport.HTTPTransport.Stop() })
	return transport.stopErr
}

var _ application.Transport = (*desktopTransport)(nil)
var _ application.TransportHTTPHandler = (*desktopTransport)(nil)
