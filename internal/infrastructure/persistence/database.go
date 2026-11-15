package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	// ErrDatabaseClosed 表示数据库已关闭，拒绝新的借用。
	ErrDatabaseClosed      = errors.New("study database is closed")
	errDatabaseUnavailable = errors.New("study database is unavailable")
)

// Database 持有当前 SQLite 句柄及其生命周期锁，所有借用都在该锁下进行。
type Database struct {
	mu               sync.RWMutex
	db               *sql.DB
	path             string
	closing          atomic.Bool
	closeErr         error
	pauseErr         error
	switchCleanupErr error
}

// NewDatabase 打开指定路径的数据库并返回其生命周期管理器。
func NewDatabase(path string) (*Database, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	return &Database{db: db, path: path}, nil
}

// AdoptDatabase 接管一个已打开且校验通过的 SQLite 句柄。调用方须在交接前完成所有使用，交接失败时自行关闭句柄。
func AdoptDatabase(db *sql.DB, path string) (*Database, error) {
	if db == nil {
		return nil, errDatabaseUnavailable
	}
	stats := db.Stats()
	if stats.Idle == 0 || stats.InUse != 0 {
		return nil, errors.New("database handoff requires an open, idle SQLite connection")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database handoff requires a file path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database handoff path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("check database handoff file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database handoff path is not a regular file: %q", absolute)
	}
	requested, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve database handoff file: %w", err)
	}

	var file string
	if err := db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		return nil, fmt.Errorf("verify SQLite main database path: %w", err)
	}
	if file == "" {
		return nil, errors.New("database handoff requires a file-backed SQLite database")
	}
	opened, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil, fmt.Errorf("resolve SQLite main database path: %w", err)
	}
	// 比较解析后的真实路径，而不是接受硬链接别名——硬链接会让同一 SQLite 文件对应不同的 journal 或 WAL 路径。
	if opened != requested {
		return nil, fmt.Errorf("SQLite main database path %q does not match handoff path %q", file, absolute)
	}
	return &Database{db: db, path: absolute}, nil
}

// WithRead 在读锁下借用句柄，直到 use 返回。回调须完成所有 SQL 操作，且不得重入 Database 或持有句柄。
func (d *Database) WithRead(use func(*sql.DB, string) error) error {
	if d.closing.Load() {
		return ErrDatabaseClosed
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.use(use)
}

// WithWrite 与 WithRead 借用约定相同，但使用写锁提供独占访问。
func (d *Database) WithWrite(use func(*sql.DB, string) error) error {
	if d.closing.Load() {
		return ErrDatabaseClosed
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.use(use)
}

func (d *Database) use(use func(*sql.DB, string) error) error {
	if d.closing.Load() {
		return ErrDatabaseClosed
	}
	if d.pauseErr != nil {
		return d.pauseErr
	}
	if d.db == nil {
		return errDatabaseUnavailable
	}
	return use(d.db, d.path)
}

// Replace 在写锁下执行连接切换。回调返回新的句柄与路径；切换报错时需携带旧库恢复所需的遗留状态。
func (d *Database) Replace(change func(*sql.DB, string) (*sql.DB, string, error)) error {
	return d.WithWrite(func(db *sql.DB, path string) error {
		next, nextPath, err := change(db, path)
		d.db, d.path = next, nextPath
		return err
	})
}

// Path 返回当前数据库文件路径。
func (d *Database) Path() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path
}

// Close 关闭当前数据库句柄；重复调用返回首次错误。
func (d *Database) Close() error {
	d.closing.Store(true)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		d.closeErr = d.db.Close()
		d.db = nil
	}
	return d.closeErr
}
