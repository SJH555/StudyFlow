package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartupMaintenanceWaitsForHomeAndCancels(t *testing.T) {
	schedule := newStartupSchedule()
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		schedule.runAfterHome(ctx, startupJob{name: "测试维护", run: func(context.Context) error {
			calls.Add(1)
			return nil
		}})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("退出后等待首页的任务未结束")
	}
	schedule.homeReady()
	schedule.homeReady()
	if calls.Load() != 0 {
		t.Fatal("取消后执行了维护")
	}
}

func TestStartupMaintenancePublishesHomeOnce(t *testing.T) {
	schedule := newStartupSchedule()
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		close(started)
		schedule.runAfterHome(context.Background(), startupJob{name: "测试维护", run: func(context.Context) error {
			close(done)
			return errors.New("非关键任务失败")
		}})
	}()
	<-started
	select {
	case <-done:
		t.Fatal("首页加载前执行了维护")
	default:
	}
	schedule.homeReady()
	schedule.homeReady()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("首页完成后未执行维护")
	}
}
