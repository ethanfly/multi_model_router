package wintray

import (
	"bytes"
	"fmt"
	"image/png"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// MenuItem represents a single tray menu entry.
type MenuItem struct {
	ID      string
	Label   string
	Checked bool
	Sep     bool // separator line
	Handler func()
}

var (
	user32   = windows.NewLazyDLL("user32.dll")
	gdi32    = windows.NewLazyDLL("gdi32.dll")
	shell32  = windows.NewLazyDLL("shell32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procCreateIconIndirect  = user32.NewProc("CreateIconIndirect")
	procGetCursorPos        = user32.NewProc("GetCursorPos")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

const (
	WM_NULL      = 0x0000
	WM_DESTROY   = 0x0002
	WM_CLOSE     = 0x0010
	WM_COMMAND   = 0x0111
	WM_USER      = 0x0400
	WM_TRAYICON  = WM_USER + 1
	WM_LBUTTONUP = 0x0202
	WM_RBUTTONUP = 0x0205

	NIM_ADD    = 0x00000000
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_GRAYED    = 0x00000001

	TPM_BOTTOMALIGN = 0x0020
	TPM_LEFTALIGN   = 0x0000

	// Base for dynamic command IDs
	cmdIDBase = 1000
)

type (
	point struct{ X, Y int32 }

	wndClassExW struct {
		CbSize        uint32
		Style         uint32
		LpfnWndProc   uintptr
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     windows.Handle
		HIcon         windows.Handle
		HCursor       windows.Handle
		HbrBackground windows.Handle
		LpszMenuName  *uint16
		LpszClassName *uint16
		HIconSm       windows.Handle
	}

	iconInfo struct {
		FIcon    int32
		XHotspot uint32
		YHotspot uint32
		HbmMask  windows.Handle
		HbmColor windows.Handle
	}

	bitmapInfoHeader struct {
		BiSize          uint32
		BiWidth         int32
		BiHeight        int32
		BiPlanes        uint16
		BiBitCount      uint16
		BiCompression   uint32
		BiSizeImage     uint32
		BiXPelsPerMeter int32
		BiYPelsPerMeter int32
		BiClrUsed       uint32
		BiClrImportant  uint32
	}

	notifyIconDataW struct {
		CbSize           uint32
		HWnd             windows.HWND
		UID              uint32
		UFlags           uint32
		UCallbackMessage uint32
		HIcon            windows.Handle
		SzTip            [128]uint16
		DwState          uint32
		DwStateMask      uint32
		SzInfo           [256]uint16
		UVersion         uint32
		SzInfoTitle      [64]uint16
		DwInfoFlags      uint32
		GuidItem         windows.GUID
		HBalloonIcon     windows.Handle
	}

	msg struct {
		HWnd    windows.HWND
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      point
	}
)

var (
	trayWnd    windows.HWND
	className  = windows.StringToUTF16Ptr("MMRTrayWnd")
	wndProcPtr = syscall.NewCallback(trayWndProc)
	inst       windows.Handle

	mu       sync.Mutex
	menuItems []MenuItem
	cmdMap   map[uintptr]func() // cmdID -> handler
)

// FNV-1a hash for menu item IDs
func fnvHash(s string) uint32 {
	h := uint32(2166136261)
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

func menuItemCmdID(id string) uintptr {
	return uintptr(cmdIDBase + fnvHash(id)%60000)
}

func trayWndProc(hwnd windows.HWND, umsg uint32, wparam uintptr, lparam uintptr) uintptr {
	switch umsg {
	case WM_TRAYICON:
		switch lparam {
		case WM_LBUTTONUP:
			mu.Lock()
			for _, item := range menuItems {
				if item.ID == "show" && item.Handler != nil {
					item.Handler()
					break
				}
			}
			mu.Unlock()
		case WM_RBUTTONUP:
			popupMenu(hwnd)
		}
	case WM_COMMAND:
		mu.Lock()
		handler := cmdMap[wparam]
		mu.Unlock()
		if handler != nil {
			handler()
		}
	case WM_DESTROY:
		removeIcon()
		procPostQuitMessage.Call(0)
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(umsg), wparam, lparam)
	return r
}

func popupMenu(hwnd windows.HWND) {
	hMenu, _, _ := procCreatePopupMenu.Call()

	mu.Lock()
	items := make([]MenuItem, len(menuItems))
	copy(items, menuItems)
	mu.Unlock()

	for _, item := range items {
		if item.Sep {
			procAppendMenuW.Call(uintptr(hMenu), MF_SEPARATOR, 0, 0)
			continue
		}
		label := windows.StringToUTF16Ptr(item.Label)
		cmdID := menuItemCmdID(item.ID)
		flags := MF_STRING
		if item.Checked {
			flags |= MF_CHECKED
		}
		procAppendMenuW.Call(uintptr(hMenu), uintptr(flags), cmdID, uintptr(unsafe.Pointer(label)))
	}

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(
		uintptr(hMenu),
		uintptr(TPM_BOTTOMALIGN|TPM_LEFTALIGN),
		uintptr(pt.X), uintptr(pt.Y),
		0, uintptr(hwnd), 0,
	)
	procPostMessageW.Call(uintptr(hwnd), WM_NULL, 0, 0)
	procDeleteObject.Call(hMenu)
}

func removeIcon() {
	if trayWnd == 0 {
		return
	}
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayWnd
	nid.UID = 1
	procShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func pngToIcon(pngData []byte) (windows.Handle, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return 0, fmt.Errorf("decode png: %w", err)
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	hdr := bitmapInfoHeader{
		BiSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		BiWidth:   int32(w),
		BiHeight:  -int32(h), // top-down DIB
		BiPlanes:  1,
		BiBitCount: 32,
	}

	var pBits unsafe.Pointer
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	colorBmp, _, _ := procCreateDIBSection.Call(
		hdc, uintptr(unsafe.Pointer(&hdr)), 0,
		uintptr(unsafe.Pointer(&pBits)), 0, 0,
	)
	if colorBmp == 0 || pBits == nil {
		procDeleteDC.Call(hdc)
		return 0, fmt.Errorf("CreateDIBSection failed")
	}

	// Write BGRA pixels
	pixels := unsafe.Slice((*byte)(pBits), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r32, g32, b32, a32 := img.At(x, y).RGBA()
			i := (y*w + x) * 4
			pixels[i+0] = byte(b32 >> 8) // B
			pixels[i+1] = byte(g32 >> 8) // G
			pixels[i+2] = byte(r32 >> 8) // R
			pixels[i+3] = byte(a32 >> 8) // A
		}
	}
	procDeleteDC.Call(hdc)

	// Monochrome mask (all opaque)
	maskBmp, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, 0)

	ii := iconInfo{
		FIcon:    1,
		HbmMask:  windows.Handle(maskBmp),
		HbmColor: windows.Handle(colorBmp),
	}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	procDeleteObject.Call(colorBmp)
	procDeleteObject.Call(maskBmp)

	if icon == 0 {
		return 0, fmt.Errorf("CreateIconIndirect failed")
	}
	return windows.Handle(icon), nil
}

// Run creates a system tray icon and blocks until Quit is called.
// Must be called in a goroutine (it calls runtime.LockOSThread internally).
func Run(tooltip string, iconPNG []byte, onShow, onQuit func()) {
	runtime.LockOSThread()

	// Set default show/quit handlers
	mu.Lock()
	menuItems = []MenuItem{
		{ID: "show", Label: "Show", Handler: onShow},
		{ID: "sep1", Sep: true},
		{ID: "quit", Label: "Quit", Handler: func() {
			removeIcon()
			onQuit()
			procPostQuitMessage.Call(0)
		}},
	}
	rebuildCmdMap()
	mu.Unlock()

	h, _, _ := procGetModuleHandleW.Call(0)
	inst = windows.Handle(h)

	// Register window class
	wcx := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   wndProcPtr,
		HInstance:     inst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))

	// Create hidden window (no WS_VISIBLE)
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("MMRTray"))),
		0,
		0, 0, 0, 0,
		0, 0, uintptr(inst), 0,
	)
	if hwnd == 0 {
		fmt.Println("wintray: CreateWindowEx failed")
		return
	}
	trayWnd = windows.HWND(hwnd)

	// Create icon from PNG
	hIcon, err := pngToIcon(iconPNG)
	if err != nil {
		fmt.Printf("wintray: icon error: %v\n", err)
		return
	}

	// Add tray icon
	tip := windows.StringToUTF16(tooltip)
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayWnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAYICON
	nid.HIcon = hIcon
	copy(nid.SzTip[:], tip)

	ret, _, _ := procShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		fmt.Println("wintray: Shell_NotifyIcon NIM_ADD failed")
		return
	}

	fmt.Println("wintray: tray icon added successfully")

	// Message loop
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// UpdateMenu replaces the tray menu items. Thread-safe.
func UpdateMenu(items []MenuItem) {
	mu.Lock()
	defer mu.Unlock()
	menuItems = make([]MenuItem, len(items))
	copy(menuItems, items)
	rebuildCmdMap()
}

func rebuildCmdMap() {
	cmdMap = make(map[uintptr]func(), len(menuItems))
	for _, item := range menuItems {
		if item.Sep || item.Handler == nil {
			continue
		}
		cmdMap[menuItemCmdID(item.ID)] = item.Handler
	}
}

// Quit removes the tray icon and exits the message loop.
func Quit() {
	if trayWnd != 0 {
		procPostMessageW.Call(uintptr(trayWnd), WM_CLOSE, 0, 0)
	}
}
