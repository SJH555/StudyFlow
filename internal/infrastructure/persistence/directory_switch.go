package persistence

import (
	"errors"
	"os"
)

var (
	// ErrDatabasePaused 表示存储位置未能确认，数据库访问已暂停。
	ErrDatabasePaused = errors.New("database access is paused: storage location could not be confirmed")
	// ErrDirectorySwitchPending 表示一次存储位置切换尚未完成清理。
	ErrDirectorySwitchPending = errors.New("a storage location change is still pending")
	// ErrDirectorySwitchUnsupported 表示当前平台不支持数据库目录切换。
	ErrDirectorySwitchUnsupported = errors.New("database directory switching is unsupported on this platform")
)

// DirectorySwitchResult 将提交状态与警告分开报告。当 Committed 为 true 时，即使设置了 Warning，新数据库仍保持激活。
type DirectorySwitchResult struct {
	Committed bool
	Retired   RetiredDatabase
	Warning   error
}

// RetiredDatabase 描述已退役的旧数据库，但不持有其句柄。CleanupAllowed 要求关闭成功且文件身份已确认；之后的任何清理都必须在触碰旧文件前重新校验 Identity。
type RetiredDatabase struct {
	Path           string
	Identity       os.FileInfo
	CloseErr       error
	CleanupAllowed bool
}
