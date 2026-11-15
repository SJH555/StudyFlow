package reminder

import (
	_ "embed"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

//go:embed assets/ringtone-063-496265.mp3
var activityRingtoneData []byte

//go:embed assets/ringtone-021-365652.mp3
var planRingtoneData []byte

//go:embed assets/ringtone-020-365650.mp3
var breakRingtoneData []byte

// Sound 表示一种提醒铃声类型。
type Sound string

const (
	// ActivitySound 活动提醒铃声。
	ActivitySound Sound = "activity"
	// PlanSound 计划提醒铃声。
	PlanSound Sound = "plan"
	// BreakSound 休息提醒铃声。
	BreakSound Sound = "break"
)

type reminderPlayer struct {
	mu         sync.Mutex
	shutdownMu sync.Mutex
	workers    sync.WaitGroup
	closed     bool
	files      map[Sound]string
	kind       Sound
	cmd        *exec.Cmd
	// volume 保存铃声音量 0–100，由桌面层在启动与设置变更时写入。
	volume int
	// expectedStop 记录由代码主动 Kill 的播放进程：等待协程对这类
	// 「signal: killed」不记日志，只报告真正异常的进程退出。
	expectedStop   map[*exec.Cmd]bool
	windowsPlaying bool
}

var player reminderPlayer

// SetVolume 更新铃声音量（0–100），越界值按边界收敛，默认 100。
func SetVolume(volume int) {
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	player.mu.Lock()
	player.volume = volume
	player.mu.Unlock()
}

// Init 将内嵌铃声写入数据目录并重置播放器状态。
func Init(dataDir string) error {
	player.shutdownMu.Lock()
	defer player.shutdownMu.Unlock()
	player.mu.Lock()
	defer player.mu.Unlock()
	player.files = map[Sound]string{
		ActivitySound: filepath.Join(dataDir, "activity-ringtone.mp3"),
		PlanSound:     filepath.Join(dataDir, "plan-ringtone.mp3"),
		BreakSound:    filepath.Join(dataDir, "break-ringtone.mp3"),
	}
	player.expectedStop = map[*exec.Cmd]bool{}
	// 音量默认 100，桌面层随后会用保存的设置覆盖。
	player.volume = 100
	if err := os.WriteFile(player.files[ActivitySound], activityRingtoneData, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(player.files[PlanSound], planRingtoneData, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(player.files[BreakSound], breakRingtoneData, 0600); err != nil {
		return err
	}
	player.closed = false
	return nil
}

// Start 播放指定类型的铃声，替换正在播放的其他铃声。
func Start(kind Sound) {
	player.mu.Lock()
	defer player.mu.Unlock()
	if player.closed {
		return
	}
	if player.cmd != nil || player.windowsPlaying {
		if player.kind == kind {
			return
		}
		if player.cmd != nil {
			player.expectedStop[player.cmd] = true
			_ = player.cmd.Process.Kill()
		}
		if player.windowsPlaying {
			stopWindowsRingtone()
			player.windowsPlaying = false
		}
		player.cmd = nil
	}
	file := player.files[kind]
	if file == "" {
		log.Printf("unknown reminder sound: %s", kind)
		return
	}
	volume := player.volume
	if runtime.GOOS == "windows" {
		if err := playWindowsRingtone(file, volume); err != nil {
			log.Printf("reminder audio failed: %v", err)
			return
		}
		player.windowsPlaying = true
		player.kind = kind
		return
	}
	if runtime.GOOS != "linux" {
		log.Print("native reminder audio is not configured for this platform")
		return
	}
	path, err := exec.LookPath("ffplay")
	if err != nil {
		log.Printf("reminder player unavailable: %v", err)
		return
	}
	cmd := exec.Command(path, "-nodisp", "-autoexit", "-loglevel", "error", "-loop", "0", "-volume", strconv.Itoa(volume), "-i", file)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("reminder audio failed: %v", err)
		return
	}
	player.cmd = cmd
	player.kind = kind
	player.workers.Add(1)
	go func() {
		defer player.workers.Done()
		err := cmd.Wait()
		player.mu.Lock()
		expected := player.expectedStop[cmd]
		delete(player.expectedStop, cmd)
		if player.cmd == cmd {
			player.cmd = nil
			player.kind = ""
		}
		player.mu.Unlock()
		if err != nil && !expected && cmd.ProcessState != nil && !cmd.ProcessState.Exited() {
			log.Printf("reminder audio stopped: %v", err)
		}
	}()
}

// Stop 停止当前播放的铃声。
func Stop() {
	StopIfKind("")
}

// StopIfKind 仅当当前播放类型与 kind 匹配时才停止；kind 为空时无条件停止。
func StopIfKind(kind Sound) {
	player.mu.Lock()
	if kind != "" && player.kind != kind {
		player.mu.Unlock()
		return
	}
	cmd := player.cmd
	if player.windowsPlaying {
		stopWindowsRingtone()
		player.windowsPlaying = false
	}
	if cmd != nil {
		player.expectedStop[cmd] = true
	}
	player.cmd = nil
	player.kind = ""
	player.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Shutdown 在桌面端生产者已排空后调用，停止播放并等待所有子进程与平台音频工作协程结束。
func Shutdown() {
	player.shutdownMu.Lock()
	defer player.shutdownMu.Unlock()
	player.mu.Lock()
	player.closed = true
	player.mu.Unlock()
	Stop()
	player.workers.Wait()
	shutdownWindowsAudio()
}

// Notify 发送系统桌面通知（当前仅 Linux 上生效）。
func Notify(title, body string) {
	player.mu.Lock()
	defer player.mu.Unlock()
	if player.closed {
		return
	}
	if runtime.GOOS != "linux" {
		return
	}
	if path, err := exec.LookPath("notify-send"); err == nil {
		cmd := exec.Command(path, "--urgency=normal", "--expire-time=5000", "--app-name=StudyFlow", title, body)
		if cmd.Start() == nil {
			player.workers.Add(1)
			go func() { defer player.workers.Done(); _ = cmd.Wait() }()
		}
	}
}
