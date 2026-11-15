package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errDesktopStopping = errors.New("应用正在退出")

// desktopTasks 仅在桌面应用运行期间接纳任务。接纳与关闭共用同一把互斥锁，
// 以避免 Wait 与新的 WaitGroup.Add 产生竞态。
type desktopTasks struct {
	mu           sync.Mutex
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	stopping     bool
	ready        bool
	drained      chan struct{} // 任务与退出准备工作已完成。
	shutdownDone chan struct{} // 协调协程（包括其 Quit 请求）已完成。
	quitPending  chan struct{} // 已排队的主线程请求仍可被取消。
	runReturned  bool
}

// secondLaunch 将获取监听与激活原生窗口操作分离。启动期间收到的多次启动请求
// 会合并为一次待执行的唤醒。
type secondLaunch struct {
	mu      sync.Mutex
	tasks   *desktopTasks
	show    func()
	pending bool
}

func (launch *secondLaunch) notify() {
	_, done, ok := launch.tasks.enter()
	if !ok {
		return
	}
	defer done()
	launch.mu.Lock()
	show := launch.show
	if show == nil {
		launch.pending = true
	}
	launch.mu.Unlock()
	if show != nil {
		show()
	}
}

func (launch *secondLaunch) activate(show func()) {
	_, done, ok := launch.tasks.enter()
	if !ok {
		return
	}
	defer done()
	launch.mu.Lock()
	launch.show = show
	pending := launch.pending
	launch.pending = false
	launch.mu.Unlock()
	if pending {
		show()
	}
}

func (tasks *desktopTasks) enter() (context.Context, func(), bool) {
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.stopping {
		return nil, nil, false
	}
	if tasks.ctx == nil {
		tasks.ctx, tasks.cancel = context.WithCancel(context.Background())
	}
	tasks.wg.Add(1)
	return tasks.ctx, tasks.wg.Done, true
}

func (tasks *desktopTasks) start(work func(context.Context)) {
	ctx, done, ok := tasks.enter()
	if !ok {
		return
	}
	go func() {
		defer done()
		work(ctx)
	}()
}

func (tasks *desktopTasks) after(delay time.Duration, work func()) {
	tasks.start(func(ctx context.Context) {
		if waitDesktopTask(ctx, delay) {
			work()
		}
	})
}

func waitDesktopTask(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (tasks *desktopTasks) stopLocked() {
	tasks.stopping = true
	if tasks.cancel != nil {
		tasks.cancel()
	}
}

func (tasks *desktopTasks) readyToQuit() bool {
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	return tasks.ready
}

// shouldQuit 绝不在 Wails 主线程上等待。只有所有已接纳的工作与准备都完成后，
// 第二次 Quit 才会透传到原生窗口销毁。
func (tasks *desktopTasks) shouldQuit(prepare, quit func()) bool {
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.ready {
		return true
	}
	if tasks.runReturned || tasks.drained != nil {
		return false
	}
	tasks.stopLocked()
	tasks.drained = make(chan struct{})
	tasks.shutdownDone = make(chan struct{})
	go func() {
		defer close(tasks.shutdownDone)
		tasks.wg.Wait()
		prepare()
		tasks.mu.Lock()
		tasks.ready = true
		close(tasks.drained)
		tasks.mu.Unlock()
		quit()
	}()
	return false
}

// quitOnMainThread 由协调协程调用。dispatch 必须只把回调入队、不等待主线程
// （即 Wails 的 InvokeAsync）。在入队期间持有 mu，可防止 Run 的回退逻辑开始后
// 再出现迟到的分发。
func (tasks *desktopTasks) quitOnMainThread(dispatch func(func()), quit func()) {
	tasks.mu.Lock()
	if tasks.runReturned {
		tasks.mu.Unlock()
		return
	}
	completed := make(chan struct{})
	tasks.quitPending = completed
	dispatch(func() {
		tasks.mu.Lock()
		if tasks.runReturned {
			tasks.mu.Unlock()
			return // 回退逻辑已确认这个被取消的请求。
		}
		tasks.quitPending = nil // 已被主线程认领，取消阶段结束。
		tasks.mu.Unlock()
		defer close(completed)
		quit()
	})
	tasks.mu.Unlock()
	<-completed
}

// stopAndWait 是启动或 Run 失败以及正常返回时的回退逻辑，必须在 Wails
// 主线程回调之外运行。
func (tasks *desktopTasks) stopAndWait() {
	tasks.mu.Lock()
	tasks.runReturned = true
	tasks.stopLocked()
	if tasks.quitPending != nil {
		close(tasks.quitPending)
		tasks.quitPending = nil
	}
	shutdownDone := tasks.shutdownDone
	tasks.mu.Unlock()
	tasks.wg.Wait()
	if shutdownDone != nil {
		<-shutdownDone
	}
}
