//go:build windows

package reminder

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var winmm = windows.NewLazySystemDLL("winmm.dll")
var mciSendString = winmm.NewProc("mciSendStringW")
var mciGetErrorString = winmm.NewProc("mciGetErrorStringW")

const reminderMCIAlias = "StudyFlowReminder"

type mciRequest struct {
	command string
	result  chan error
}

var mciMu sync.Mutex
var mciRequests chan mciRequest
var mciWorkerDone chan struct{}

func mciCommand(command string) error {
	mciMu.Lock()
	defer mciMu.Unlock()
	if mciRequests == nil {
		mciRequests = make(chan mciRequest)
		mciWorkerDone = make(chan struct{})
		go func() {
			defer close(mciWorkerDone)
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			for request := range mciRequests {
				request.result <- sendMCICommand(request.command)
			}
		}()
	}
	result := make(chan error, 1)
	mciRequests <- mciRequest{command: command, result: result}
	return <-result
}

func sendMCICommand(command string) error {
	wide, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return err
	}
	code, _, callErr := mciSendString.Call(uintptr(unsafe.Pointer(wide)), 0, 0, 0)
	if code == 0 {
		return nil
	}
	message := make([]uint16, 256)
	if ok, _, _ := mciGetErrorString.Call(code, uintptr(unsafe.Pointer(&message[0])), uintptr(len(message))); ok != 0 {
		return fmt.Errorf("MCI %s: %s", command, windows.UTF16ToString(message))
	}
	return fmt.Errorf("MCI %s failed (%d): %v", command, code, callErr)
}

// playWindowsRingtone 用 MCI 循环播放铃声，音量映射到 MCI 的 0–1000 刻度。
func playWindowsRingtone(path string, volume int) error {
	if err := mciCommand(fmt.Sprintf(`open "%s" type mpegvideo alias %s`, path, reminderMCIAlias)); err != nil {
		return err
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	if err := mciCommand(fmt.Sprintf("setaudio %s volume to %d", reminderMCIAlias, volume*10)); err != nil {
		_ = mciCommand("close " + reminderMCIAlias)
		return err
	}
	if err := mciCommand("play " + reminderMCIAlias + " repeat"); err != nil {
		_ = mciCommand("close " + reminderMCIAlias)
		return err
	}
	return nil
}

func stopWindowsRingtone() {
	stopErr := mciCommand("stop " + reminderMCIAlias + " wait")
	closeErr := mciCommand("close " + reminderMCIAlias + " wait")
	if stopErr != nil || closeErr != nil {
		log.Printf("停止提醒铃声失败: stop=%v close=%v", stopErr, closeErr)
		if err := mciCommand("close all wait"); err != nil {
			log.Printf("关闭音频设备失败: %v", err)
		}
	}
}

func shutdownWindowsAudio() {
	mciMu.Lock()
	defer mciMu.Unlock()
	if mciRequests != nil {
		close(mciRequests)
		<-mciWorkerDone
		mciRequests = nil
	}
}
