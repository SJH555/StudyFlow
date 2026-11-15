//go:build (linux && !android) || windows

package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	pendingLocationName   = "storage-location.txt.pending"
	pendingVersion        = 1
	locationNoReplace     = 1
	locationRegular       = 0x8000
	locationDirectory     = 0x4000
	locationTempPrefix    = ".studyflow-location-"
	locationCleanupSuffix = ".cleanup"
	maxLocationBytes      = 64 * 1024
)

// LocationOutcome 表示存储位置切换的结算结果。
type LocationOutcome uint8

const (
	// LocationUncertain 表示切换结果未能确认。
	LocationUncertain LocationOutcome = iota
	// LocationUnchanged 表示位置保持旧值。
	LocationUnchanged
	// LocationCommitted 表示新位置已生效。
	LocationCommitted
)

// LocationResolution 描述一次切换结算后的结果与对应数据库路径。
type LocationResolution struct {
	Outcome      LocationOutcome
	DatabasePath string
}

type locationIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Birth  string `json:"birth,omitempty"`
}

type locationState struct {
	State string           `json:"state"`
	ID    locationIdentity `json:"identity,omitzero"`
	Bytes []byte           `json:"bytes,omitempty"`
}

type locationDatabase struct {
	Path string           `json:"path"`
	ID   locationIdentity `json:"identity"`
}

type locationRecord struct {
	Version             int                 `json:"version"`
	OperationID         string              `json:"operationId"`
	PositionDirectoryID locationIdentity    `json:"positionDirectoryId"`
	OldLocation         locationState       `json:"oldLocation"`
	NewLocation         locationState       `json:"newLocation"`
	TempName            string              `json:"tempName"`
	OldDatabase         locationDatabase    `json:"oldDatabase"`
	TargetDatabase      locationDatabase    `json:"targetDatabase"`
	Retirement          *locationRetirement `json:"retirement,omitempty"`
}

type locationIO struct {
	write       func(*os.File, []byte) (int, error)
	syncFile    func(*os.File) error
	closeFile   func(*os.File) error
	syncDir     func(*os.File) error
	rename      func(int, string, int, string, uint) error
	moveOwned   func(int, string, int, string, uint) error
	unlinkOwned func(int, string, int) error
}

// LocationSwitch 持有一个打开的目录句柄。调用方在数据库生命周期锁下串行化访问，并在使用后关闭。
type LocationSwitch struct {
	dir             *os.File
	path            string
	name            string
	dirID           locationIdentity
	old             locationState
	record          *locationRecord
	marker          locationState
	ready           bool
	removed         bool
	settled         LocationOutcome
	io              locationIO
	retirementPaths []string
}

// OpenLocationSwitch 打开存储位置文件并解析其待处理恢复记录（若存在）。
func OpenLocationSwitch(path string) (_ *LocationSwitch, resultErr error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if name == pendingLocationName || name == pendingLocationName+locationCleanupSuffix || strings.HasPrefix(name, locationTempPrefix) || name == "/" {
		return nil, fmt.Errorf("invalid storage location filename: %s", name)
	}
	parent := filepath.Dir(path)
	dir, id, err := openLocationDirectory(parent)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, dir.Close())
		}
	}()
	s := &LocationSwitch{dir: dir, path: parent, name: name, dirID: id, io: defaultLocationIO()}
	s.old, err = s.read(name, maxLocationBytes)
	if err != nil {
		return nil, err
	}
	s.marker, _, err = s.readMarker()
	if err != nil {
		return nil, err
	}
	if s.marker.State != "missing" {
		var record locationRecord
		if err := json.Unmarshal(s.marker.Bytes, &record); err != nil {
			return nil, fmt.Errorf("decode pending storage location: %w", err)
		}
		if !record.valid() || record.PositionDirectoryID != s.dirID {
			return nil, errors.New("invalid pending storage location record or directory identity")
		}
		s.record = &record
	}
	return s, nil
}

// Close 关闭持有的目录句柄。
func (s *LocationSwitch) Close() error { return s.dir.Close() }

// HasPending 在存在待处理记录时返回 true，即使发布结果无法确认。
func (s *LocationSwitch) HasPending() bool { return s.record != nil }

// CheckSwitchTarget 保留现有单行位置格式及安装目录的历史语义。若路径解析会改变将要写入的目录，需传入规范化后的目标。
func CheckSwitchTarget(target, installDir string) error {
	if !validLocationPath(target) {
		return fmt.Errorf("数据目录无法按现有格式保存: %q", target)
	}
	if installDir == "" || filepath.Clean(target) == filepath.Clean(installDir) {
		return fmt.Errorf("不能将安装目录用作新的数据目录: %s", target)
	}
	return nil
}

func validLocationPath(path string) bool {
	return utf8.ValidString(path) && filepath.IsAbs(path) && strings.TrimSpace(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

// Stage 在 Publish 触及正式名称前持久化恢复记录。出错时，若 HasPending 为 true，调用方必须保留两个数据库。
func (s *LocationSwitch) Stage(oldDBPath, targetDBPath, installDir string) (resultErr error) {
	if s.HasPending() {
		return errors.New("a storage location change is already pending")
	}
	if err := s.checkOld(); err != nil {
		return err
	}
	oldDB, err := identifyLocationDatabase(oldDBPath)
	if err != nil {
		return err
	}
	targetDB, err := identifyLocationDatabase(targetDBPath)
	if err != nil {
		return err
	}
	if oldDB.ID == targetDB.ID {
		return errors.New("source and target database have the same identity")
	}
	if err := CheckSwitchTarget(filepath.Dir(targetDB.Path), installDir); err != nil {
		return err
	}
	record := locationRecord{Version: pendingVersion, OperationID: rand.Text(), PositionDirectoryID: s.dirID,
		OldLocation: s.old, OldDatabase: oldDB, TargetDatabase: targetDB}
	if s.retirementPaths != nil {
		record.Retirement, err = planLocationRetirement(oldDB, targetDB, s.retirementPaths)
		if err != nil {
			return err
		}
	}
	record.TempName = locationTempPrefix + record.OperationID
	newBytes := []byte(filepath.Dir(targetDB.Path) + "\n")
	record.NewLocation, err = s.writeTemp(record.TempName, newBytes)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil && !s.HasPending() {
			resultErr = errors.Join(resultErr, s.removeOwned(record.TempName, record.NewLocation, false))
		}
	}()
	if !record.valid() {
		return errors.New("invalid storage location change record")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data) > maxLocationBytes*4 {
		return errors.New("pending storage location record is too large")
	}
	markerTemp := record.TempName + "-pending"
	marker, err := s.writeTemp(markerTemp, data)
	if err != nil {
		return err
	}
	publishErr := s.rename(markerTemp, pendingLocationName, locationNoReplace)
	current, readErr := s.read(pendingLocationName, maxLocationBytes*4)
	if readErr != nil || sameLocationState(current, marker) || publishErr == nil {
		s.record, s.marker = &record, marker
	}
	if readErr != nil || !sameLocationState(current, marker) {
		var cleanupErr error
		if !s.HasPending() {
			cleanupErr = s.removeOwned(markerTemp, marker, true)
		}
		return errors.Join(publishErr, readErr, errors.New("pending storage location publication could not be confirmed"), cleanupErr)
	}
	if err := s.syncDirectory(); err != nil {
		return errors.Join(publishErr, err)
	}
	s.ready = true
	return nil
}

// Publish 在最后一次预检发现外部修改时绝不改动正式名称；替换本身没有基于身份的 compare-and-swap。
func (s *LocationSwitch) Publish() (LocationResolution, error) {
	if !s.ready {
		return LocationResolution{}, errors.New("pending storage location is not ready for publication")
	}
	s.ready = false
	if err := s.checkMarker(false); err != nil {
		return LocationResolution{}, err
	}
	staged, err := s.read(s.record.TempName, maxLocationBytes)
	if err != nil || !sameLocationState(staged, s.record.NewLocation) {
		return LocationResolution{}, errors.Join(err, errors.New("staged storage location changed"))
	}
	if err := s.record.TargetDatabase.check(); err != nil {
		return LocationResolution{}, err
	}
	if err := s.checkOld(); err != nil {
		return LocationResolution{}, err
	}
	flags := uint(0)
	if s.old.State == "missing" {
		flags = locationNoReplace
	}
	publishErr := s.rename(s.record.TempName, s.name, flags)
	result, err := s.settle(publishErr != nil)
	return result, errors.Join(publishErr, err)
}

// Recover 仅分类磁盘状态。启动调用方须另行确认所选数据库可用；对于隐式旧位置，还须确认现有位置规则仍会选中它。Recover 不删除任何内容。
func (s *LocationSwitch) Recover() (LocationResolution, error) {
	if !s.HasPending() {
		return LocationResolution{}, errors.New("no pending storage location change")
	}
	return s.settle(true)
}

// Clear 移除已验证的待处理记录；若位置未变化，还会移除已验证的临时位置文件。它绝不删除数据库或备份文件。
func (s *LocationSwitch) Clear() error {
	if s.settled == LocationUncertain {
		return errors.New("cannot clear an unresolved storage location change")
	}
	s.ready = false
	if !s.removed {
		result, err := s.observe(s.settled == LocationUnchanged)
		if err != nil || result.Outcome != s.settled {
			return errors.Join(err, errors.New("storage location changed before clearing its pending record"))
		}
		if s.settled == LocationUnchanged {
			// 在临时文件删除持久化之前，保持记录可恢复。
			if err := s.removeOwned(s.record.TempName, s.record.NewLocation, true); err != nil {
				return err
			}
			if err := s.syncDirectory(); err != nil {
				return err
			}
			confirmed, err := s.observe(true)
			if err != nil || confirmed != result {
				return errors.Join(err, errors.New("storage location changed during temporary file cleanup"))
			}
		}
		if err := s.removeOwned(pendingLocationName, s.marker, false); err != nil {
			return err
		}
		s.removed = true
	}
	marker, _, err := s.readMarker()
	if err != nil || marker.State != "missing" {
		return errors.Join(err, errors.New("pending storage location name was reused after removal"))
	}
	if err := s.syncDirectory(); err != nil {
		return err
	}
	if s.settled == LocationCommitted {
		s.old = s.record.NewLocation
	}
	s.record, s.removed, s.ready, s.settled = nil, false, false, LocationUncertain
	return nil
}

func (s *LocationSwitch) settle(allowOld bool) (LocationResolution, error) {
	s.settled = LocationUncertain
	result, err := s.observe(allowOld)
	if err != nil {
		return LocationResolution{}, err
	}
	if err := s.syncDirectory(); err != nil {
		return LocationResolution{}, err
	}
	confirmed, err := s.observe(allowOld)
	if err != nil || confirmed != result {
		return LocationResolution{}, errors.Join(err, errors.New("storage location changed during directory synchronization"))
	}
	s.settled = result.Outcome
	return result, nil
}

func (s *LocationSwitch) observe(allowOld bool) (LocationResolution, error) {
	if err := s.checkMarker(true); err != nil {
		return LocationResolution{}, err
	}
	current, err := s.read(s.name, maxLocationBytes)
	if err != nil {
		return LocationResolution{}, err
	}
	var result LocationResolution
	var database locationDatabase
	switch {
	case sameLocationState(current, s.record.NewLocation):
		result.Outcome, database = LocationCommitted, s.record.TargetDatabase
	case allowOld && sameLocationState(current, s.record.OldLocation):
		result.Outcome, database = LocationUnchanged, s.record.OldDatabase
	default:
		return LocationResolution{}, errors.New("official storage location does not match the pending record")
	}
	if err := database.check(); err != nil {
		return LocationResolution{}, err
	}
	result.DatabasePath = database.Path
	return result, nil
}

func (s *LocationSwitch) checkOld() error {
	if err := s.checkDirectory(); err != nil {
		return err
	}
	current, err := s.read(s.name, maxLocationBytes)
	if err != nil || !sameLocationState(current, s.old) {
		return errors.Join(err, errors.New("official storage location changed before publication"))
	}
	return nil
}

func (s *LocationSwitch) checkMarker(allowIsolated bool) error {
	if err := s.checkDirectory(); err != nil {
		return err
	}
	marker, isolated, err := s.readMarker()
	if err != nil || (isolated && !allowIsolated) || !sameLocationState(marker, s.marker) {
		return errors.Join(err, errors.New("pending storage location record changed"))
	}
	return nil
}

func (s *LocationSwitch) readMarker() (locationState, bool, error) {
	marker, err := s.read(pendingLocationName, maxLocationBytes*4)
	if err != nil {
		return locationState{}, false, err
	}
	isolated, err := s.read(pendingLocationName+locationCleanupSuffix, maxLocationBytes*4)
	if err != nil || isolated.State == "missing" {
		return marker, false, err
	}
	if marker.State != "missing" {
		return locationState{}, false, errors.New("both pending storage location names are occupied")
	}
	// 被中断的清理可能把记录留在隔离名称下。
	return isolated, true, nil
}

func sameLocationState(a, b locationState) bool {
	return a.State == b.State && a.ID == b.ID && bytes.Equal(a.Bytes, b.Bytes)
}

func (s locationState) valid() bool {
	switch s.State {
	case "missing":
		return s.ID == (locationIdentity{}) && len(s.Bytes) == 0
	case "empty":
		return s.ID.Inode != 0 && len(s.Bytes) == 0
	case "content":
		return s.ID.Inode != 0 && len(s.Bytes) > 0 && len(s.Bytes) <= maxLocationBytes
	default:
		return false
	}
}

func (r locationRecord) valid() bool {
	return r.Version == pendingVersion && r.OperationID != "" &&
		filepath.Base(r.OperationID) == r.OperationID &&
		filepath.Base(r.TempName) == r.TempName && r.TempName == locationTempPrefix+r.OperationID &&
		r.PositionDirectoryID.Inode != 0 && r.OldLocation.valid() && r.NewLocation.valid() &&
		r.NewLocation.State == "content" && r.OldDatabase.valid() && r.TargetDatabase.valid() &&
		r.OldDatabase.ID != r.TargetDatabase.ID &&
		validLocationPath(filepath.Dir(r.TargetDatabase.Path)) &&
		bytes.Equal(r.NewLocation.Bytes, []byte(filepath.Dir(r.TargetDatabase.Path)+"\n")) &&
		(r.Retirement == nil || r.Retirement.valid())
}

func (d locationDatabase) valid() bool {
	// JSON 会把字符串中的非法 UTF-8 替换掉，因此拒绝无法从待处理记录逐字节还原的路径。
	return utf8.ValidString(d.Path) && filepath.IsAbs(d.Path) && filepath.Clean(d.Path) == d.Path &&
		filepath.Base(d.Path) == "study.db" && d.ID.Inode != 0
}

func (d locationDatabase) check() error {
	current, err := identifyLocationDatabase(d.Path)
	if err != nil || current.ID != d.ID {
		return errors.Join(err, fmt.Errorf("database identity could not be confirmed: %s", d.Path))
	}
	return nil
}

func identifyLocationDatabase(path string) (locationDatabase, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return locationDatabase{}, err
	}
	file, err := openLocationRead(nil, path)
	if err != nil {
		return locationDatabase{}, err
	}
	id, err := getLocationIdentity(file, locationRegular)
	if err := errors.Join(err, file.Close()); err != nil {
		return locationDatabase{}, err
	}
	database := locationDatabase{Path: path, ID: id}
	if !database.valid() {
		return locationDatabase{}, fmt.Errorf("invalid switch database: %s", path)
	}
	return database, nil
}

func (s *LocationSwitch) read(name string, limit int64) (locationState, error) {
	file, err := openLocationRead(s.dir, name)
	if errors.Is(err, os.ErrNotExist) {
		return locationState{State: "missing"}, nil
	}
	if err != nil {
		return locationState{}, err
	}
	id, err := getLocationIdentity(file, locationRegular)
	if err != nil {
		return locationState{}, errors.Join(err, file.Close())
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err := errors.Join(err, file.Close()); err != nil {
		return locationState{}, err
	}
	if int64(len(data)) > limit {
		return locationState{}, fmt.Errorf("storage location file is too large: %s", name)
	}
	state := "content"
	if len(data) == 0 {
		state = "empty"
	}
	return locationState{State: state, ID: id, Bytes: data}, nil
}

func (s *LocationSwitch) writeTemp(name string, data []byte) (locationState, error) {
	file, err := createLocationFile(s.dir, name)
	if err != nil {
		return locationState{}, err
	}
	id, err := getLocationIdentity(file, locationRegular)
	if err != nil {
		return locationState{}, errors.Join(err, s.io.closeFile(file))
	}
	n, err := s.io.write(file, data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = s.io.syncFile(file)
	}
	if err := errors.Join(err, s.io.closeFile(file)); err != nil {
		current, readErr := s.read(name, maxLocationBytes*4)
		if readErr == nil && current.State != "missing" {
			if current.ID == id {
				readErr = s.removeOwned(name, current, true)
			} else {
				readErr = fmt.Errorf("staged storage location file identity changed: %s", name)
			}
		}
		return locationState{}, errors.Join(fmt.Errorf("stage %s: %w", name, err), readErr)
	}
	return locationState{State: "content", ID: id, Bytes: data}, nil
}

func (s *LocationSwitch) removeOwned(name string, expected locationState, missingOK bool) error {
	// 使用稳定的隔离名称，使恢复能够重试在 unlink 前被中断的移动；绝不在两个名称下替换已存在的条目。
	isolatedName := name + locationCleanupSuffix
	isolated, err := s.read(isolatedName, maxLocationBytes*4)
	if err != nil {
		return err
	}
	current, err := s.read(name, maxLocationBytes*4)
	if err != nil {
		return err
	}
	fd := int(s.dir.Fd())
	if isolated.State == "missing" {
		if missingOK && current.State == "missing" {
			return nil
		}
		if !sameLocationState(current, expected) {
			return fmt.Errorf("storage location file ownership changed: %s", name)
		}
		if err := s.io.moveOwned(fd, name, fd, isolatedName, locationNoReplace); err != nil {
			return fmt.Errorf("isolate storage location file %s: %w", name, err)
		}
		isolated, err = s.read(isolatedName, maxLocationBytes*4)
		if err != nil || !sameLocationState(isolated, expected) {
			return s.restoreOwned(isolatedName, name, errors.Join(err, fmt.Errorf("isolated storage location file ownership changed: %s", isolatedName)))
		}
	} else if current.State != "missing" || !sameLocationState(isolated, expected) {
		return fmt.Errorf("cannot confirm ownership of isolated storage location file: %s", isolatedName)
	}
	// 文件移除仍是按名称进行。能够访问隔离名称的外部写入者可能在最后一次身份检查与 unlink 之间竞争。
	if err := s.io.unlinkOwned(fd, isolatedName, 0); err != nil {
		cleanupErr := fmt.Errorf("remove isolated storage location file %s: %w", isolatedName, err)
		current, readErr := s.read(isolatedName, maxLocationBytes*4)
		if readErr != nil || !sameLocationState(current, expected) {
			return errors.Join(cleanupErr, readErr)
		}
		return s.restoreOwned(isolatedName, name, cleanupErr)
	}
	return nil
}

func (s *LocationSwitch) restoreOwned(isolatedName, name string, cause error) error {
	fd := int(s.dir.Fd())
	if err := s.io.moveOwned(fd, isolatedName, fd, name, locationNoReplace); err != nil {
		return errors.Join(cause, fmt.Errorf("restore %s from %s without replacing an entry: %w", name, isolatedName, err))
	}
	return cause
}

func (s *LocationSwitch) rename(old, next string, flags uint) error {
	fd := int(s.dir.Fd())
	return s.io.rename(fd, old, fd, next, flags)
}

func (s *LocationSwitch) checkDirectory() error {
	dir, id, err := openLocationDirectory(s.path)
	if err != nil {
		return err
	}
	if id != s.dirID {
		err = errors.New("storage location directory identity changed")
	}
	return errors.Join(err, dir.Close())
}

func (s *LocationSwitch) syncDirectory() error {
	if err := s.checkDirectory(); err != nil {
		return err
	}
	if err := s.io.syncDir(s.dir); err != nil {
		return fmt.Errorf("sync storage location directory %s: %w", s.path, err)
	}
	return s.checkDirectory()
}
