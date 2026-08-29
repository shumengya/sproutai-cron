//go:build windows

package install

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func ensureUserPath(binDir string) (added bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	cur, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, err
	}

	binNorm := strings.TrimRight(strings.ToLower(filepath.Clean(binDir)), `\`)
	var parts []string
	if strings.TrimSpace(cur) != "" {
		for _, p := range strings.Split(cur, ";") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.TrimRight(strings.ToLower(filepath.Clean(p)), `\`) == binNorm {
				return false, nil // already present
			}
			parts = append(parts, p)
		}
	}
	newPath := binDir
	if len(parts) > 0 {
		newPath = binDir + ";" + strings.Join(parts, ";")
	}
	if err := k.SetStringValue("Path", newPath); err != nil {
		return false, err
	}
	broadcastEnvChange()
	return true, nil
}

func setUserEnv(key, value string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(key, value); err != nil {
		return err
	}
	broadcastEnvChange()
	return nil
}

func broadcastEnvChange() {
	const HWND_BROADCAST = 0xffff
	const WM_SETTINGCHANGE = 0x001A
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	env, _ := syscall.UTF16PtrFromString("Environment")
	_, _, _ = proc.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(env)),
		0x0002, // SMTO_ABORTIFHUNG
		5000,
		0,
	)
}
