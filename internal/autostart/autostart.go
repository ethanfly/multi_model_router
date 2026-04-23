package autostart

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const regKeyPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
const appName = "MultiModelRouter"

var (
	advapi32 = windows.NewLazyDLL("advapi32.dll")

	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")

	appNameUTF16 = windows.StringToUTF16Ptr(appName)
	keyPathUTF16 = windows.StringToUTF16Ptr(regKeyPath)
)

const (
	KEY_QUERY_VALUE = 0x0001
	KEY_SET_VALUE   = 0x0002
	REG_SZ          = 1
)

// Enable adds the application to Windows startup registry.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	var key windows.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		uintptr(windows.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPathUTF16)),
		0, KEY_SET_VALUE,
		uintptr(unsafe.Pointer(&key)),
	)
	if ret != 0 {
		return fmt.Errorf("RegOpenKeyEx failed: %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(key))

	exeUTF16 := windows.StringToUTF16(exe)
	ret, _, _ = procRegSetValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(appNameUTF16)),
		0, REG_SZ,
		uintptr(unsafe.Pointer(&exeUTF16[0])),
		uintptr(len(exeUTF16)*2),
	)
	if ret != 0 {
		return fmt.Errorf("RegSetValueEx failed: %d", ret)
	}
	return nil
}

// Disable removes the application from Windows startup registry.
func Disable() error {
	var key windows.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		uintptr(windows.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPathUTF16)),
		0, KEY_SET_VALUE,
		uintptr(unsafe.Pointer(&key)),
	)
	if ret != 0 {
		return fmt.Errorf("RegOpenKeyEx failed: %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(key))

	ret, _, _ = procRegDeleteValueW.Call(uintptr(key), uintptr(unsafe.Pointer(appNameUTF16)))
	if ret != 0 {
		return fmt.Errorf("RegDeleteValue failed: %d", ret)
	}
	return nil
}

// IsEnabled checks if the application is in the Windows startup registry.
func IsEnabled() bool {
	var key windows.Handle
	ret, _, _ := procRegOpenKeyExW.Call(
		uintptr(windows.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPathUTF16)),
		0, KEY_QUERY_VALUE,
		uintptr(unsafe.Pointer(&key)),
	)
	if ret != 0 {
		return false
	}
	defer procRegCloseKey.Call(uintptr(key))

	var bufLen uint32
	ret, _, _ = procRegQueryValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(appNameUTF16)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&bufLen)),
	)
	return ret == 0
}
