package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// startupSchedule 以首页加载完成信号安排维护任务，不占用数据库连接等待前端。
// 信号只发布一次，托盘启动也由工作区加载流程发布，不依赖窗口焦点。
type startupSchedule struct {
	once  sync.Once
	ready chan struct{}
}

func newStartupSchedule() *startupSchedule {
	return &startupSchedule{ready: make(chan struct{})}
}

func (s *startupSchedule) homeReady() {
	s.once.Do(func() { close(s.ready) })
}

type startupJob struct {
	name string
	run  func(context.Context) error
}

func (s *startupSchedule) runAfterHome(ctx context.Context, job startupJob) {
	waiting := time.Now()
	select {
	case <-ctx.Done():
		return
	case <-s.ready:
	}
	if ctx.Err() != nil {
		return
	}
	log.Printf("启动任务 %s 等待首页 %s", job.name, time.Since(waiting))
	measureStartup(job.name, func() error { return job.run(ctx) })
}

func measureStartup(name string, work func() error) error {
	started := time.Now()
	err := work()
	log.Printf("启动任务 %s 执行 %s，错误=%v", name, time.Since(started), err)
	return err
}
