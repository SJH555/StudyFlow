package main

import (
	"context"
	"log"
	"time"
)

func autoBackupLoop(ctx context.Context, store *App) {
	check := func() {
		if err := store.scheduler.WithContext(ctx).BackupCheck(time.Now()); err != nil {
			log.Printf("自动备份失败: %v", err)
		}
	}
	if ctx.Err() != nil {
		return
	}
	_ = measureStartup("首次备份检查", func() error {
		return store.scheduler.WithContext(ctx).BackupCheck(time.Now())
	})
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() == nil {
				check()
			}
		}
	}
}
