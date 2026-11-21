package transfer

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exam_notify/internal/infrastructure/persistence"
)

const (
	Extension       = ".studyflow"
	formatVersion   = 1
	maxManifestSize = 16 << 10
	// 限制外部文件的展开大小，避免损坏或伪造的包耗尽磁盘。
	maxDatabaseSize = 16 << 30
)

type manifest struct {
	Format          int    `json:"format"`
	DatabaseVersion int64  `json:"databaseVersion"`
	ExportedAt      string `json:"exportedAt"`
	TimeZone        string `json:"timeZone"`
	SHA256          string `json:"sha256"`
}

func writeArchive(ctx context.Context, path, snapshot string, version int64, now time.Time) error {
	input, err := os.Open(snapshot)
	if err != nil {
		return err
	}
	defer input.Close()

	return persistence.WriteNewArchive(ctx, path, func(output *os.File) error {
		zipOut := zip.NewWriter(output)
		database, err := zipOut.Create("study.db")
		if err != nil {
			return err
		}
		digest := sha256.New()
		if _, err := io.Copy(io.MultiWriter(database, digest), contextReader{ctx, input}); err != nil {
			return err
		}
		entry, err := zipOut.Create("manifest.json")
		if err != nil {
			return err
		}
		zone, _ := now.Zone()
		info := manifest{
			Format: formatVersion, DatabaseVersion: version,
			ExportedAt: now.Format(time.RFC3339), TimeZone: zone,
			SHA256: hex.EncodeToString(digest.Sum(nil)),
		}
		if err := json.NewEncoder(entry).Encode(info); err != nil {
			return err
		}
		return zipOut.Close()
	})
}

func readArchive(ctx context.Context, source, snapshot string) (manifest, error) {
	var info manifest
	zipIn, err := zip.OpenReader(source)
	if err != nil {
		return info, fmt.Errorf("无法读取 StudyFlow 备份包: %w", err)
	}
	defer zipIn.Close()

	var database, index *zip.File
	if len(zipIn.File) != 2 {
		return info, errors.New("备份包必须包含数据库和清单")
	}
	for _, file := range zipIn.File {
		if !file.Mode().IsRegular() {
			return info, errors.New("备份包包含非普通文件")
		}
		switch file.Name {
		case "study.db":
			database = file
		case "manifest.json":
			index = file
		}
	}
	if database == nil || index == nil || index.UncompressedSize64 > maxManifestSize ||
		database.UncompressedSize64 == 0 || database.UncompressedSize64 > maxDatabaseSize {
		return info, errors.New("备份包成员或大小无效")
	}

	reader, err := index.Open()
	if err != nil {
		return info, err
	}
	contents, readErr := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return info, err
	}
	if len(contents) > maxManifestSize || json.Unmarshal(contents, &info) != nil {
		return manifest{}, errors.New("备份清单无法解析")
	}
	if info.Format != formatVersion {
		return info, errors.New("不支持此备份包格式，请使用支持该格式的 StudyFlow")
	}
	if _, err := time.Parse(time.RFC3339, info.ExportedAt); err != nil || info.DatabaseVersion < 1 {
		return info, errors.New("备份清单中的时间或数据库版本无效")
	}

	data, err := database.Open()
	if err != nil {
		return info, err
	}
	defer data.Close()
	output, err := os.OpenFile(snapshot, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return info, err
	}
	digest := sha256.New()
	input := io.LimitReader(contextReader{ctx, data}, maxDatabaseSize+1)
	size, copyErr := io.Copy(io.MultiWriter(output, digest), input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return info, err
	}
	if size != int64(database.UncompressedSize64) || size > maxDatabaseSize ||
		!strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), info.SHA256) {
		return info, errors.New("备份包的数据库大小或校验值不符")
	}
	return info, nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func checkSourceFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxDatabaseSize {
		return fmt.Errorf("文件为空、过大或不是普通文件: %s", filepath.Base(path))
	}
	return nil
}
