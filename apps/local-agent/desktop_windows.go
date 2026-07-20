//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"sync"

	"github.com/getlantern/systray"
	"github.com/ksamwang/acrunu-fast-aicut/internal/localagent"
	"golang.org/x/sys/windows"
)

const localAgentMutexName = `Local\ACRUNUFastCutLocalAgent`

//go:embed tray.ico
var trayIcon []byte

func runLocalAgent(server *localagent.Server, logger *slog.Logger) error {
	mutex, alreadyRunning, err := acquireSingleInstanceMutex()
	if err != nil {
		return err
	}
	if alreadyRunning {
		_ = windows.CloseHandle(mutex)
		return nil
	}
	defer windows.CloseHandle(mutex)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverDone := make(chan error, 1)
	var startOnce sync.Once

	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("ACRUNU预处理程序")
		systray.SetTooltip("ACRUNU预处理程序")
		quitItem := systray.AddMenuItem("退出", "退出 ACRUNU预处理程序")

		startOnce.Do(func() {
			go func() {
				serverDone <- server.RunContext(ctx)
				systray.Quit()
			}()
		})
		go func() {
			<-quitItem.ClickedCh
			systray.Quit()
		}()
	}, func() {
		cancel()
	})

	err = <-serverDone
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("local agent stopped", "error", err)
		return err
	}
	return nil
}

func acquireSingleInstanceMutex() (windows.Handle, bool, error) {
	name, err := windows.UTF16PtrFromString(localAgentMutexName)
	if err != nil {
		return 0, false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return handle, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	return handle, false, nil
}
