package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type preparedDirectory struct {
	databasePath  string
	sameDirectory bool
	resources     []preparedResource
	sourceBackups []string
}

type preparedResource struct {
	path          string
	info          os.FileInfo
	checkMetadata bool
}

type directoryPrepareIO struct {
	copyBackup    func(io.Writer, io.Reader) (int64, error)
	syncFile      func(*os.File) error
	syncDirectory func(string) error
}

// prepareDirectory 需要在当前数据库上独占使用。它不触碰当前句柄与位置文件；成功后由调用方持有准备的文件，并可在提交切换前用 cleanup 丢弃。
func prepareDirectory(ctx context.Context, db *sql.DB, sourcePath, target, locationFile string) (*preparedDirectory, error) {
	return prepareDirectoryWithIO(ctx, db, sourcePath, target, locationFile, directoryPrepareIO{
		copyBackup:    io.Copy,
		syncFile:      (*os.File).Sync,
		syncDirectory: syncPreparedDirectory,
	})
}

func prepareDirectoryWithIO(ctx context.Context, db *sql.DB, sourcePath, target, locationFile string, fileIO directoryPrepareIO) (_ *preparedDirectory, resultErr error) {
	paths, err := checkDirectoryPaths(sourcePath, target, locationFile)
	if err != nil {
		return nil, err
	}
	prepared := &preparedDirectory{
		databasePath:  filepath.Join(paths.target, "study.db"),
		sameDirectory: paths.sameDirectory,
	}
	if prepared.sameDirectory {
		return prepared, nil
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, prepared.cleanup())
		}
	}()
	if err := prepared.makeDirectory(paths.target); err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(paths.target, ".studyflow-snapshot-")
	if err != nil {
		return nil, err
	}
	work, err := prepared.recordDirectory(workDir)
	if err != nil {
		return nil, err
	}
	// 在 SQLite 打开之前先关闭保留文件，Windows 上同样如此。
	staged, err := prepared.stageFile(workDir, nil)
	if err != nil {
		return nil, err
	}
	if err := snapshotDatabase(ctx, db, staged.path); err != nil {
		return nil, fmt.Errorf("创建数据库快照 %s: %w", staged.path, err)
	}
	if err := staged.syncFile(); err != nil {
		return nil, fmt.Errorf("同步数据库快照 %s: %w", staged.path, err)
	}
	if err := prepared.publish(staged, prepared.databasePath); err != nil {
		return nil, fmt.Errorf("发布目标数据库 %s: %w", prepared.databasePath, err)
	}
	if err := work.remove(); err != nil {
		return nil, err
	}
	if err := prepared.copyBackups(paths.sourceBackups, paths.targetBackups, fileIO); err != nil {
		return nil, fmt.Errorf("迁移备份文件失败: %w", err)
	}
	if err := prepared.syncDirectories(paths.target, paths.targetBackups, fileIO.syncDirectory); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (p *preparedDirectory) copyBackups(source, target string, fileIO directoryPrepareIO) error {
	if err := checkCanonicalDirectory(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := p.makeDirectory(target); err != nil {
		return err
	}
	for _, entry := range entries {
		// 保留既有备份选择规则：跳过目录与其他后缀。清理另行确认源文件的身份与内容。
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		from, to := filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())
		staged, err := p.stageFile(target, func(file *os.File) error {
			return copyBackupFile(from, file, fileIO)
		})
		if err != nil {
			return fmt.Errorf("复制备份 %s: %w", from, err)
		}
		if err := p.publish(staged, to); err != nil {
			return fmt.Errorf("发布备份 %s: %w", to, err)
		}
		p.sourceBackups = append(p.sourceBackups, from)
	}
	return nil
}

func copyBackupFile(source string, target *os.File, fileIO directoryPrepareIO) error {
	// 在 Unix 上，不要为等待 FIFO 写入者而在检查文件类型前阻塞。
	file, err := os.OpenFile(source, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	before, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if !before.Mode().IsRegular() {
		return errors.Join(fmt.Errorf("备份源不是普通文件: %s", source), file.Close())
	}
	copied, err := fileIO.copyBackup(target, file)
	if err == nil {
		if syncErr := fileIO.syncFile(target); syncErr != nil {
			err = fmt.Errorf("同步目标备份文件 %s: %w", target.Name(), syncErr)
		}
	}
	after, statErr := file.Stat()
	if err := errors.Join(err, statErr, file.Close()); err != nil {
		return err
	}
	// Stat 与 os.Open 遵循相同的符号链接规则。关闭后再复核路径，使 Windows 能在无共享冲突的情况下解析文件 ID。
	current, err := os.Stat(source)
	if err != nil {
		return err
	}
	// 这些检查用于发现可观察的身份与元数据变化。并发写入者即使还原元数据，仍可能改变已复制的内容。
	if !sameBackupFile(before, after) || !sameBackupFile(after, current) || copied != before.Size() {
		return fmt.Errorf("备份文件在复制期间发生变化: %s", source)
	}
	return nil
}

func sameBackupFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Mode() == after.Mode() &&
		before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func (p *preparedDirectory) stageFile(dir string, write func(*os.File) error) (preparedResource, error) {
	file, err := os.CreateTemp(dir, ".studyflow-*.tmp")
	if err != nil {
		return preparedResource{}, err
	}
	info, err := file.Stat()
	resource := preparedResource{path: file.Name(), info: info}
	p.resources = append(p.resources, resource)
	if err == nil && write != nil {
		err = write(file)
	}
	return resource, errors.Join(err, file.Close())
}

func (p *preparedDirectory) publish(staged preparedResource, target string) error {
	if err := staged.checkIdentity(); err != nil {
		return err
	}
	if err := checkCanonicalDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	publishErr := publishPreparedFile(staged.path, target)
	// 即使发布报错也保留暂存身份。若最终文件名现在指向我们的文件，将其纳入回滚是安全的。
	final := preparedResource{path: target, info: staged.info}
	if info, err := final.currentInfo(); err == nil {
		final.info = info
		final.checkMetadata = true
		p.resources = append(p.resources, final)
	} else if publishErr == nil {
		return err
	}
	if publishErr != nil {
		return publishErr
	}
	return staged.remove()
}

func (p *preparedDirectory) syncDirectories(target, backups string, syncDirectory func(string) error) error {
	paths := []string{backups, target}
	for i := len(p.resources) - 1; i >= 0; i-- {
		resource := p.resources[i]
		if resource.info != nil && resource.info.IsDir() {
			paths = append(paths, filepath.Dir(resource.path))
		}
	}
	synced := make(map[string]bool)
	for _, path := range paths {
		if synced[path] {
			continue
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) && path == backups {
			continue
		}
		if err := checkCanonicalDirectory(path); err != nil {
			return err
		}
		if err := syncDirectory(path); err != nil {
			return fmt.Errorf("同步目标目录 %s: %w", path, err)
		}
		synced[path] = true
	}
	return nil
}
