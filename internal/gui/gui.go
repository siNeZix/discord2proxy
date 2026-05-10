package gui

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"discord-szx/internal/config"
	"discord-szx/internal/deploy"
	"discord-szx/internal/discord"
	"discord-szx/internal/proxy"
)

func guiLog(msg string) {
	f, _ := os.OpenFile("gui.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		fmt.Fprintf(f, "%s\n", msg)
	}
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	procGetModuleHandle       = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassEx       = user32.NewProc("RegisterClassExW")
	procCreateWindowEx        = user32.NewProc("CreateWindowExW")
	procGetMessage            = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessage       = user32.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procDefWindowProc         = user32.NewProc("DefWindowProcW")
	procDestroyWindow         = user32.NewProc("DestroyWindow")
	procGetClientRect         = user32.NewProc("GetClientRect")
	procInvalidateRect        = user32.NewProc("InvalidateRect")
	procSetWindowText         = user32.NewProc("SetWindowTextW")
	procGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procSetWindowRgn          = user32.NewProc("SetWindowRgn")
	procSetLayeredWindowAttrs = user32.NewProc("SetLayeredWindowAttributes")
	procShowWindow            = user32.NewProc("ShowWindow")
	procUpdateWindow          = user32.NewProc("UpdateWindow")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procSetCapture            = user32.NewProc("SetCapture")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procScreenToClient        = user32.NewProc("ScreenToClient")
	procBeginPaint            = user32.NewProc("BeginPaint")
	procEndPaint              = user32.NewProc("EndPaint")
	procTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	procDrawTextW             = user32.NewProc("DrawTextW")
	procSendMessage           = user32.NewProc("SendMessageW")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procSetCursor             = user32.NewProc("SetCursor")
	procLoadCursor            = user32.NewProc("LoadCursorW")
	procSetTimer              = user32.NewProc("SetTimer")
	procKillTimer             = user32.NewProc("KillTimer")

	gdiCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	gdiDeleteObject        = gdi32.NewProc("DeleteObject")
	gdiSelectObject        = gdi32.NewProc("SelectObject")
	gdiCreatePen           = gdi32.NewProc("CreatePen")
	gdiRoundRect           = gdi32.NewProc("RoundRect")
	gdiRectangle           = gdi32.NewProc("Rectangle")
	gdiSetTextColor        = gdi32.NewProc("SetTextColor")
	gdiSetBkMode           = gdi32.NewProc("SetBkMode")
	gdiCreateFontIndirectW = gdi32.NewProc("CreateFontIndirectW")
	gdiCreateRoundRectRgn  = gdi32.NewProc("CreateRoundRectRgn")
	gdiGradientFill        = gdi32.NewProc("GradientFill")
	gdiFillRect            = user32.NewProc("FillRect")
	gdiMoveToEx            = gdi32.NewProc("MoveToEx")
	gdiLineTo              = gdi32.NewProc("LineTo")
	gdiEllipse             = gdi32.NewProc("Ellipse")
	gdiCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	gdiCreateCompatibleBitmap= gdi32.NewProc("CreateCompatibleBitmap")
	gdiBitBlt              = gdi32.NewProc("BitBlt")
	gdiDeleteDC            = gdi32.NewProc("DeleteDC")
)

const (
	WS_POPUP     = 0x80000000
	WS_VISIBLE     = 0x10000000
	WS_MINIMIZEBOX = 0x00020000
	WS_CAPTION     = 0x00C00000
	WS_SYSMENU     = 0x00080000
	WS_CHILD       = 0x40000000
	WS_TABSTOP     = 0x00010000

	SW_SHOW       = 5
	SWP_FRAMECHANGED = 0x0020
	SWP_NOMOVE    = 0x0002
	SWP_NOSIZE    = 0x0001
	SWP_NOZORDER  = 0x0004
	SWP_SHOWWINDOW= 0x0040
	SWP_NOACTIVATE= 0x0010

	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	IDC_ARROW        = 32512
	IDI_APPLICATION  = 32512

	WM_CREATE          = 0x0001
	WM_DESTROY         = 0x0002
	WM_CLOSE           = 0x0010
	WM_PAINT           = 0x000F
	WM_MOUSEMOVE       = 0x0200
	WM_LBUTTONDOWN     = 0x0201
	WM_LBUTTONUP       = 0x0202
	WM_MOUSELEAVE      = 0x02A3
	WM_NCHITTEST       = 0x0084
	WM_TIMER           = 0x0113

	HTCAPTION  = 2
	HTCLIENT   = 1
	HTNOWHERE  = 0
	HTCLOSE    = 20

	TRANSPARENT = 1
	OPAQUE      = 2

	LWA_ALPHA    = 0x00000002
	LWA_COLORKEY = 0x00000001

	DT_LEFT       = 0x00000000
	DT_CENTER     = 0x00000001
	DT_RIGHT      = 0x00000002
	DT_VCENTER    = 0x00000004
	DT_SINGLELINE = 0x00000020
	DT_NOCLIP     = 0x00000100

	TME_LEAVE = 0x00000002
	TME_HOVER = 0x00000001

	// Custom colors — dark modern theme
	colBg        uint32 = 0x1E1E2E
	colBg2       uint32 = 0x252535
	colAccent    uint32 = 0xA97BFF
	colAccent2   uint32 = 0x7B5EB5
	colText      uint32 = 0xFFFFFF
	colTextDim   uint32 = 0xA0A0B0
	colGreen     uint32 = 0x4ADE80
	colRed       uint32 = 0xF87171
	colBtnGrad1  uint32 = 0xA97BFF
	colBtnGrad2  uint32 = 0x7B5EB5
	colBtnHover1 uint32 = 0xB88FFF
	colBtnHover2 uint32 = 0x8A6FC5
	colClose     uint32 = 0xF87171
	colCloseHover uint32 = 0xFF9999
)

const (
	winW   = 420
	winH   = 310
	radius = 12
	className   = "DiscordSzxModern"
	windowTitle = "discord-szx"
)

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type paintStruct struct {
	HDC         uintptr
	FErase      int32
	_           [4]byte // padding before RECT to 8-byte align? Actually after 4-byte int32, RECT (4-byte aligned) needs no padding. But C struct pads to 8 at end.
	RcPaint     rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
	_           [4]byte // pad to 72 bytes (multiple of 8)
}

type trackMouseEvent struct {
	CbSize      uint32
	DwFlags     uint32
	HWndTrack   uintptr
	DwHoverTime uint32
}

type logFont struct {
	Height         int32
	Width          int32
	Escapement     int32
	Orientation    int32
	Weight         int32
	Italic         byte
	Underline      byte
	StrikeOut      byte
	CharSet        byte
	OutPrecision   byte
	ClipPrecision  byte
	Quality        byte
	PitchAndFamily byte
	FaceName       [32]uint16
}

type triVertex struct {
	X     int32
	Y     int32
	Red   uint16
	Green uint16
	Blue  uint16
	Alpha uint16
}

type gradientRect struct {
	UpperLeft  uint32
	LowerRight uint32
}

// rectHit — rectangle for hit-testing
type rectHit struct {
	X, Y, W, H int32
}

func (r rectHit) contains(x, y int32) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

type App struct {
	cfg  *config.Config
	hWnd uintptr

	install   *discord.DiscordInstall
	proxyInfo *proxy.ProxyInfo
	discordErr error
	proxyErr   error
	installed  bool

	// Mouse tracking
	mouseX, mouseY int32
	hoverInstall   bool
	hoverUninstall bool
	hoverClose     bool
	tracking       bool

	// Geometry
	btnInstall   rectHit
	btnUninstall rectHit
	btnClose     rectHit
	titleBar     rectHit

	canInstall   bool
	canUninstall bool

	statusMsg   string
	statusColor uint32
}

func Run() {
	guiLog("=== Run start ===")
	cfg := config.Default()
	app := &App{cfg: cfg}

	install, err := discord.FindPrimaryDiscord(cfg)
	if err != nil {
		app.discordErr = err
		guiLog("discord err: " + err.Error())
	} else {
		app.install = install
		app.installed = deploy.IsInstalled(install, cfg)
		guiLog("discord found: " + install.Channel)
	}

	proxyInfo, err := proxy.DetectBestProxy("127.0.0.1", cfg.ProxyPorts)
	if err != nil {
		app.proxyErr = err
		guiLog("proxy err: " + err.Error())
	} else {
		app.proxyInfo = proxyInfo
		guiLog("proxy found")
	}

	app.canInstall = app.install != nil && app.proxyInfo != nil && !app.installed
	app.canUninstall = app.install != nil && app.installed

	hInstance, _, _ := procGetModuleHandle.Call(0)
	guiLog(fmt.Sprintf("hInstance=%d", hInstance))

	classNamePtr, _ := syscall.UTF16PtrFromString(className)

	wndProc := syscall.NewCallback(app.wndProc)

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   wndProc,
		HInstance:     hInstance,
		HbrBackground: 0,
		LpszClassName: classNamePtr,
	}

	atom, _, errReg := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	guiLog(fmt.Sprintf("RegisterClassEx atom=%d err=%v", atom, errReg))
	if atom == 0 {
		guiLog("RegisterClassEx failed, aborting")
		return
	}

	cx, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	cy, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	x := (int(cx) - winW) / 2
	y := (int(cy) - winH) / 2
	if x < 0 { x = 0 }
	if y < 0 { y = 0 }

	guiLog(fmt.Sprintf("creating window at %d,%d size %dx%d", x, y, winW, winH))

	titlePtr, _ := syscall.UTF16PtrFromString(windowTitle)

	WS_EX_APPWINDOW := uintptr(0x00040000)
	hWnd, _, errCreate := procCreateWindowEx.Call(
		WS_EX_APPWINDOW,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(WS_POPUP|WS_VISIBLE|WS_MINIMIZEBOX|WS_SYSMENU),
		uintptr(x), uintptr(y),
		uintptr(winW), uintptr(winH),
		0, 0, hInstance, 0,
	)
	guiLog(fmt.Sprintf("CreateWindowEx hwnd=%d err=%v", hWnd, errCreate))
	if hWnd == 0 {
		guiLog("CreateWindowEx failed, aborting")
		return
	}
	app.hWnd = hWnd
	guiLog("before ShowWindow")
	procShowWindow.Call(hWnd, uintptr(SW_SHOW))
	guiLog("before UpdateWindow")
	procUpdateWindow.Call(hWnd)
	guiLog("before SetForegroundWindow")
	procSetForegroundWindow.Call(hWnd)
	guiLog("ShowWindow/UpdateWindow/SetForegroundWindow called")

	// Round window corners
	rgn, _, _ := gdiCreateRoundRectRgn.Call(0, 0, uintptr(winW+1), uintptr(winH+1), uintptr(radius*2), uintptr(radius*2))
	if rgn != 0 {
		procSetWindowRgn.Call(hWnd, rgn, 1)
	}

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			guiLog("GetMessage ret=0, exiting loop")
			break
		}
		if int32(ret) == -1 {
			guiLog("GetMessage ret=-1, error")
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	guiLog("=== Run end ===")
}

func (app *App) wndProc(hWnd, uMsg, wParam, lParam uintptr) uintptr {
	switch uint32(uMsg) {
	case WM_CREATE:
		return 0

	case WM_PAINT:
		guiLog("WM_PAINT received")
		app.onPaint(hWnd)
		guiLog("WM_PAINT done")
		return 0

	case WM_MOUSEMOVE:
		mx := int32(int16(lParam & 0xFFFF))
		my := int32(int16((lParam >> 16) & 0xFFFF))
		app.mouseX = mx
		app.mouseY = my
		app.updateHover(mx, my)
		if !app.tracking {
			app.tracking = true
			tme := trackMouseEvent{
				CbSize:    uint32(unsafe.Sizeof(trackMouseEvent{})),
				DwFlags:   TME_LEAVE,
				HWndTrack: hWnd,
			}
			procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		}
		return 0

	case WM_MOUSELEAVE:
		app.tracking = false
		app.hoverInstall = false
		app.hoverUninstall = false
		app.hoverClose = false
		procInvalidateRect.Call(hWnd, 0, 0)
		return 0

	case WM_LBUTTONDOWN:
		mx := int32(int16(lParam & 0xFFFF))
		my := int32(int16((lParam >> 16) & 0xFFFF))
		if app.btnClose.contains(mx, my) {
			procDestroyWindow.Call(hWnd)
			return 0
		}
		if app.btnInstall.contains(mx, my) && app.canInstall {
			app.onInstall()
			return 0
		}
		if app.btnUninstall.contains(mx, my) && app.canUninstall {
			app.onUninstall()
			return 0
		}
		if app.titleBar.contains(mx, my) {
			procReleaseCapture.Call()
			// Simulate dragging via WM_NCLBUTTONDOWN + HTCAPTION
			procSendMessage.Call(hWnd, 0x00A1, HTCAPTION, 0) // WM_NCLBUTTONDOWN
		}
		return 0

	case WM_TIMER:
		app.statusMsg = ""
		procKillTimer.Call(hWnd, 1)
		procInvalidateRect.Call(hWnd, 0, 0)
		return 0

	case WM_NCHITTEST:
		ret, _, _ := procDefWindowProc.Call(hWnd, uMsg, wParam, lParam)
		return ret

	case 0x0020: // WM_SETCURSOR
		cursor, _, _ := procLoadCursor.Call(0, IDC_ARROW)
		procSetCursor.Call(cursor)
		return 1

	case WM_CLOSE:
		procDestroyWindow.Call(hWnd)
		return 0

	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hWnd, uMsg, wParam, lParam)
	return ret
}

func (app *App) updateHover(mx, my int32) {
	oldHover := app.hoverInstall || app.hoverUninstall || app.hoverClose
	app.hoverInstall = app.btnInstall.contains(mx, my) && app.canInstall
	app.hoverUninstall = app.btnUninstall.contains(mx, my) && app.canUninstall
	app.hoverClose = app.btnClose.contains(mx, my)
	newHover := app.hoverInstall || app.hoverUninstall || app.hoverClose
	if oldHover != newHover {
		procInvalidateRect.Call(app.hWnd, 0, 0)
	}
}

func (app *App) onPaint(hWnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hWnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	procGetClientRect.Call(hWnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top

	// Background
	brushBg, _, _ := gdiCreateSolidBrush.Call(uintptr(colBg))
	gdiFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brushBg)
	gdiDeleteObject.Call(brushBg)

	// --- Title bar ---
	titleH := int32(40)
	app.titleBar = rectHit{0, 0, w, titleH}

	// Title text
	hFontTitle := createFont(18, 600, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontTitle)
	gdiSetTextColor.Call(hdc, uintptr(colText))
	gdiSetBkMode.Call(hdc, TRANSPARENT)
	drawText(hdc, 20, 8, 200, 28, "discord-szx", DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFontTitle)

	// Close button
	closeSz := int32(28)
	app.btnClose = rectHit{w - closeSz - 12, (titleH - closeSz) / 2, closeSz, closeSz}
	closeCol := colClose
	if app.hoverClose {
		closeCol = colCloseHover
	}
	drawRoundRect(hdc, app.btnClose.X, app.btnClose.Y, app.btnClose.W, app.btnClose.H, 6, closeCol)
	hFontIcon := createFont(14, 400, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontIcon)
	gdiSetTextColor.Call(hdc, uintptr(colText))
	drawText(hdc, app.btnClose.X, app.btnClose.Y, app.btnClose.W, app.btnClose.H, "×", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFontIcon)

	// Divider line
	penLine, _, _ := gdiCreatePen.Call(0, 1, uintptr(0x3A3A4A))
	gdiSelectObject.Call(hdc, penLine)
	gdiMoveToEx.Call(hdc, uintptr(20), uintptr(titleH), 0)
	gdiLineTo.Call(hdc, uintptr(w-20), uintptr(titleH))
	gdiDeleteObject.Call(penLine)

	// --- Status cards ---
	cardY := int32(52)
	cardH := int32(42)
	cardMargin := int32(14)
	cardW := (w - 40 - cardMargin*2) / 3

	statuses := []struct {
		label  string
		value  string
		ok     bool
	}{
		{"DISCORD", app.discordStatusShort(), app.discordErr == nil},
		{"PROXY", app.proxyStatusShort(), app.proxyErr == nil},
		{"STATUS", app.installStatusShort(), app.installed},
	}

	for i, s := range statuses {
		cx := int32(20 + int32(i)*(cardW+cardMargin))
		drawStatusCard(hdc, cx, cardY, cardW, cardH, s.label, s.value, s.ok)
	}

	// --- Buttons ---
	btnY := cardY + cardH + 28
	btnW := int32(140)
	btnH := int32(40)
	btnSpacing := int32(24)
	totalBtnW := btnW*2 + btnSpacing
	btnX := (w - totalBtnW) / 2

	app.btnInstall = rectHit{btnX, btnY, btnW, btnH}
	app.btnUninstall = rectHit{btnX + btnW + btnSpacing, btnY, btnW, btnH}

	canInstall := app.install != nil && app.proxyInfo != nil && !app.installed
	canUninstall := app.install != nil && app.installed
	app.canInstall = canInstall
	app.canUninstall = canUninstall

	drawButton(hdc, app.btnInstall, "Установить", app.hoverInstall && canInstall, canInstall)
	drawButton(hdc, app.btnUninstall, "Удалить", app.hoverUninstall && canUninstall, canUninstall)

	// --- Status banner ---
	if app.statusMsg != "" {
		bannerY := btnY + btnH + 10
		bannerH := int32(24)
		drawRoundRect(hdc, 20, bannerY, w-40, bannerH, 6, colBg2)
		hFontBanner := createFont(12, 600, "Segoe UI")
		gdiSelectObject.Call(hdc, hFontBanner)
		gdiSetTextColor.Call(hdc, uintptr(app.statusColor))
		gdiSetBkMode.Call(hdc, TRANSPARENT)
		drawText(hdc, 20, bannerY, w-40, bannerH, app.statusMsg, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		gdiDeleteObject.Call(hFontBanner)
	}

	// --- Footer ---
	footerY := h - 30
	hFontFoot := createFont(12, 400, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontFoot)
	gdiSetTextColor.Call(hdc, uintptr(colTextDim))
	drawText(hdc, 0, footerY, w, 20, "by siNeZix", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFontFoot)

	// --- Version / detail text ---
	detailY := cardY + cardH + 6
	hFontDetail := createFont(11, 400, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontDetail)
	gdiSetTextColor.Call(hdc, uintptr(colTextDim))
	if app.install != nil {
		drawText(hdc, 20, detailY, w-40, 18, fmt.Sprintf("Найден: %s (%s)", app.install.Channel, app.install.Version), DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	} else {
		drawText(hdc, 20, detailY, w-40, 18, "Discord не найден", DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
	gdiDeleteObject.Call(hFontDetail)

	procEndPaint.Call(hWnd, uintptr(unsafe.Pointer(&ps)))
}

func drawStatusCard(hdc uintptr, x, y, w, h int32, label, value string, ok bool) {
	// Card background
	drawRoundRect(hdc, x, y, w, h, 8, colBg2)

	// Indicator dot
	dotR := int32(6)
	dotCol := colRed
	if ok {
		dotCol = colGreen
	}
	dotX := x + w - dotR - 12
	dotY := y + (h-dotR)/2
	drawFilledEllipse(hdc, dotX, dotY, dotR, dotR, dotCol)

	// Label
	hFontLabel := createFont(10, 600, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontLabel)
	gdiSetTextColor.Call(hdc, uintptr(colTextDim))
	gdiSetBkMode.Call(hdc, TRANSPARENT)
	drawText(hdc, x+12, y+6, w-30, 16, label, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFontLabel)

	// Value
	hFontValue := createFont(13, 600, "Segoe UI")
	gdiSelectObject.Call(hdc, hFontValue)
	gdiSetTextColor.Call(hdc, uintptr(colText))
	drawText(hdc, x+12, y+20, w-30, 18, value, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFontValue)
}

func drawButton(hdc uintptr, r rectHit, text string, hover, enabled bool) {
	c1, c2 := colBtnGrad1, colBtnGrad2
	if hover && enabled {
		c1, c2 = colBtnHover1, colBtnHover2
	}
	if !enabled {
		c1, c2 = 0x3A3A4A, 0x30303A
	}

	drawGradientRoundRect(hdc, r.X, r.Y, r.W, r.H, 8, c1, c2)

	hFont := createFont(13, 600, "Segoe UI")
	gdiSelectObject.Call(hdc, hFont)
	if enabled {
		gdiSetTextColor.Call(hdc, uintptr(colText))
	} else {
		gdiSetTextColor.Call(hdc, uintptr(0x707080))
	}
	gdiSetBkMode.Call(hdc, TRANSPARENT)
	drawText(hdc, r.X, r.Y, r.W, r.H, text, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	gdiDeleteObject.Call(hFont)
}

func drawGradientRoundRect(hdc uintptr, x, y, w, h, rad int32, c1, c2 uint32) {
	// Fallback: draw solid rounded rect with average color if GradientFill unavailable
	avg := blendColor(c1, c2, 0.5)
	drawRoundRect(hdc, x, y, w, h, rad, avg)
}

func drawRoundRect(hdc uintptr, x, y, w, h, rad int32, col uint32) {
	brush, _, _ := gdiCreateSolidBrush.Call(uintptr(col))
	pen, _, _ := gdiCreatePen.Call(0, 1, uintptr(col))
	oldBrush, _, _ := gdiSelectObject.Call(hdc, brush)
	oldPen, _, _ := gdiSelectObject.Call(hdc, pen)
	gdiRoundRect.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), uintptr(rad*2), uintptr(rad*2))
	gdiSelectObject.Call(hdc, oldPen)
	gdiSelectObject.Call(hdc, oldBrush)
	gdiDeleteObject.Call(pen)
	gdiDeleteObject.Call(brush)
}

func drawFilledEllipse(hdc uintptr, x, y, w, h int32, col uint32) {
	brush, _, _ := gdiCreateSolidBrush.Call(uintptr(col))
	pen, _, _ := gdiCreatePen.Call(0, 1, uintptr(col))
	oldBrush, _, _ := gdiSelectObject.Call(hdc, brush)
	oldPen, _, _ := gdiSelectObject.Call(hdc, pen)
	// Use Ellipse
	gdiEllipse.Call(hdc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h))
	gdiSelectObject.Call(hdc, oldPen)
	gdiSelectObject.Call(hdc, oldBrush)
	gdiDeleteObject.Call(pen)
	gdiDeleteObject.Call(brush)
}

func drawText(hdc uintptr, x, y, w, h int32, text string, flags uint32) {
	r := rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
	textPtr, _ := syscall.UTF16PtrFromString(text)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(textPtr)), uintptr(len([]rune(text))), uintptr(unsafe.Pointer(&r)), uintptr(flags))
}

func createFont(height, weight int32, face string) uintptr {
	var lf logFont
	lf.Height = -int32((int64(height) * 96) / 72) // points to logical units approx
	if lf.Height > -4 {
		lf.Height = -height
	}
	lf.Weight = weight
	lf.CharSet = 1 // DEFAULT_CHARSET
	lf.Quality = 5 // CLEARTYPE_QUALITY
	copy(lf.FaceName[:], utf16FromString(face))
	hFont, _, _ := gdiCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return hFont
}

func utf16FromString(s string) []uint16 {
	p, _ := syscall.UTF16FromString(s)
	return p[:len(p)-1]
}

func blendColor(c1, c2 uint32, t float64) uint32 {
	r1 := (c1 >> 16) & 0xFF
	g1 := (c1 >> 8) & 0xFF
	b1 := c1 & 0xFF
	r2 := (c2 >> 16) & 0xFF
	g2 := (c2 >> 8) & 0xFF
	b2 := c2 & 0xFF
	r := uint32(float64(r1)*(1-t) + float64(r2)*t)
	g := uint32(float64(g1)*(1-t) + float64(g2)*t)
	b := uint32(float64(b1)*(1-t) + float64(b2)*t)
	return (r << 16) | (g << 8) | b
}

func (app *App) discordStatusShort() string {
	if app.discordErr != nil {
		return "Нет"
	}
	return "OK"
}

func (app *App) proxyStatusShort() string {
	if app.proxyErr != nil {
		return "Нет"
	}
	return "OK"
}

func (app *App) installStatusShort() string {
	if app.installed {
		return "Активно"
	}
	return "Неактивно"
}

func (app *App) onInstall() {
	if app.install == nil || app.proxyInfo == nil {
		return
	}

	d := deploy.New(app.cfg, false, false)
	if err := d.Deploy(app.install, app.proxyInfo); err != nil {
		app.showStatus(fmt.Sprintf("Ошибка установки: %v", err), false)
		return
	}
	if err := d.Verify(app.install); err != nil {
		app.showStatus(fmt.Sprintf("Ошибка проверки: %v", err), false)
		return
	}

	app.installed = true
	app.canInstall = false
	app.canUninstall = true
	procInvalidateRect.Call(app.hWnd, 0, 0)
	app.showStatus("Прокси установлен! Перезапустите Discord.", true)
}

func (app *App) onUninstall() {
	if app.install == nil {
		return
	}

	d := deploy.New(app.cfg, false, false)
	if err := d.Uninstall(app.install); err != nil {
		app.showStatus(fmt.Sprintf("Ошибка удаления: %v", err), false)
		return
	}

	app.installed = false
	app.canInstall = app.proxyInfo != nil
	app.canUninstall = false
	procInvalidateRect.Call(app.hWnd, 0, 0)
	app.showStatus("Прокси удалён! Перезапустите Discord.", true)
}

func (app *App) showStatus(msg string, ok bool) {
	app.statusMsg = msg
	if ok {
		app.statusColor = colGreen
	} else {
		app.statusColor = colRed
	}
	procSetTimer.Call(app.hWnd, 1, 5000, 0)
	procInvalidateRect.Call(app.hWnd, 0, 0)
}
