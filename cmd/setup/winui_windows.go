//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"discord-szx/internal/config"
	"discord-szx/internal/installer"
)

// ---- Win32 bindings -------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetModuleHandle      = kernel32.NewProc("GetModuleHandleW")
	procLoadImage            = user32.NewProc("LoadImageW")
	procRegisterClassEx      = user32.NewProc("RegisterClassExW")
	procCreateWindowEx       = user32.NewProc("CreateWindowExW")
	procDefWindowProc        = user32.NewProc("DefWindowProcW")
	procGetMessage           = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessage      = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procBeginPaint           = user32.NewProc("BeginPaint")
	procEndPaint             = user32.NewProc("EndPaint")
	procFillRect             = user32.NewProc("FillRect")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procInvalidateRect       = user32.NewProc("InvalidateRect")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procSendMessage          = user32.NewProc("SendMessageW")
	procPostMessage          = user32.NewProc("PostMessageW")
	procEnableWindow         = user32.NewProc("EnableWindow")
	procSetTimer             = user32.NewProc("SetTimer")
	procKillTimer            = user32.NewProc("KillTimer")
	procDrawText             = user32.NewProc("DrawTextW")
	procSetBkMode            = gdi32.NewProc("SetBkMode")
	procSetTextColor         = gdi32.NewProc("SetTextColor")
	procCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	procCreateFontIndirect   = gdi32.NewProc("CreateFontIndirectW")
	procCreateRoundRectRgn   = gdi32.NewProc("CreateRoundRectRgn")
	procSelectObject         = gdi32.NewProc("SelectObject")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procRoundRect            = gdi32.NewProc("RoundRect")
	procSetDCBrushColor      = gdi32.NewProc("SetDCBrushColor")
	procSetDCPenColor        = gdi32.NewProc("SetDCPenColor")
	procGetStockObject       = gdi32.NewProc("GetStockObject")
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type rect struct {
	left, top, right, bottom int32
}

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type drawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   uintptr
	hDC        uintptr
	rcItem     rect
	itemData   uintptr
}

type logFont struct {
	height         int32
	width          int32
	escapement     int32
	orientation    int32
	weight         int32
	italic         byte
	underline      byte
	strikeOut      byte
	charSet        byte
	outPrecision   byte
	clipPrecision  byte
	quality        byte
	pitchAndFamily byte
	faceName       [32]uint16
}

const (
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	wsTabStop     = 0x00010000

	bsAutoCheckBox = 0x00000003
	bsOwnerDraw    = 0x0000000B

	bmGetCheck = 0x00F0
	bmSetCheck = 0x00F1
	bstChecked = 1

	wmDestroy        = 0x0002
	wmPaint          = 0x000F
	wmCommand        = 0x0111
	wmCtlColorStatic = 0x0138
	wmCtlColorBtn    = 0x0135
	wmDrawItem       = 0x002B
	wmSetFont        = 0x0030
	wmTimer          = 0x0113

	// Timer that animates the indeterminate progress bar (unknown size).
	timerPulse    = 1
	pulsePeriodMs = 60

	bnClicked = 0

	swShow = 5
	swHide = 0

	// Window-class icon loaded from the embedded resource (rsrc_windows_*.syso),
	// whose RT_GROUP_ICON is generated with numeric ID #1 by go-winres.
	imageIcon     = 1          // IMAGE_ICON
	lrDefaultSize = 0x00000040 // LR_DEFAULTSIZE
	lrShared      = 0x00008000 // LR_SHARED
	iconResID     = 1          // embedded icon group resource ID

	odsDisabled = 0x0004

	transparent  = 1
	dtCenter     = 0x00000001
	dtVcenter    = 0x00000004
	dtSingleLine = 0x00000020
	dtLeft       = 0x00000000
	dtWordBreak  = 0x00000010

	smCxScreen = 0
	smCyScreen = 1

	spiGetWorkArea = 0x0030

	dcBrush = 18
	dcPen   = 19

	fwNormal = 400
	fwBold   = 700

	defaultCharset    = 1
	clipDefaultPrecis = 0
	outDefaultPrecis  = 0
	cleartypeQuality  = 5
	variablePitch     = 2

	// Custom messages posted from the worker goroutine.
	wmAppProgress = 0x8000 + 1 // WM_APP+1
	wmAppDone     = 0x8000 + 2 // WM_APP+2
)

// ---- theme (mirrors internal/gui colors) ----------------------------------

// colorref packs an NRGBA-style color into a Win32 COLORREF (0x00BBGGRR).
func colorref(r, g, b byte) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

var (
	clrBg      = colorref(0x1E, 0x1E, 0x2E)
	clrBg2     = colorref(0x25, 0x25, 0x35)
	clrAccent  = colorref(0xA9, 0x7B, 0xFF)
	clrAccent2 = colorref(0x7B, 0x5E, 0xB5)
	clrText    = colorref(0xFF, 0xFF, 0xFF)
	clrTextDim = colorref(0xA0, 0xA0, 0xB0)
	clrGreen   = colorref(0x4A, 0xDE, 0x80)
	clrRed     = colorref(0xF8, 0x71, 0x71)
)

// ---- window state ---------------------------------------------------------

type setupWindow struct {
	hwnd      uintptr
	hInst     uintptr
	hbrBg     uintptr
	hbrBg2    uintptr
	hbrAccent uintptr
	fontBase  uintptr
	fontH     uintptr
	fontBtn   uintptr

	hInstallBtn uintptr
	hDesktopChk uintptr
	hStartChk   uintptr

	mu          sync.Mutex
	busy        bool
	installing  bool // controls hidden during install (only status+bar shown)
	statusMsg   string
	statusColor uintptr
	progress    float64 // 0..1, <0 means indeterminate (unknown size)
	showBar     bool
	pulse       float64 // 0..1 animation phase for the indeterminate bar
	done        bool    // install succeeded; main loop should relaunch+exit
}

var win *setupWindow

const (
	idInstall  = 1001
	idDesktop  = 1002
	idStartMnu = 1003
)

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) uint16 { return uint16(v & 0xffff) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xffff) }

func makeFont(height int32, weight int32) uintptr {
	lf := logFont{
		height:         height,
		weight:         weight,
		charSet:        defaultCharset,
		outPrecision:   outDefaultPrecis,
		clipPrecision:  clipDefaultPrecis,
		quality:        cleartypeQuality,
		pitchAndFamily: variablePitch,
	}
	for i, r := range syscall.StringToUTF16("Segoe UI") {
		if i >= len(lf.faceName) {
			break
		}
		lf.faceName[i] = r
	}
	h, _, _ := procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf)))
	return h
}

// runWindow creates and runs the installer window, then (on success) relaunches
// the installed copy. Blocks until the window is closed.
func runWindow() {
	// The window, its message pump and the WndProc callback must all run on the
	// same OS thread. Without locking, the Go scheduler may migrate this
	// goroutine to another thread, orphaning the window's message queue and
	// causing the UI to freeze (especially once cross-thread PostMessage calls
	// from the worker goroutine arrive).
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandle.Call(0)

	win = &setupWindow{
		hInst:       hInst,
		statusColor: clrTextDim,
		progress:    -1,
	}
	win.hbrBg, _, _ = procCreateSolidBrush.Call(clrBg)
	win.hbrBg2, _, _ = procCreateSolidBrush.Call(clrBg2)
	win.hbrAccent, _, _ = procCreateSolidBrush.Call(clrAccent)
	win.fontBase = makeFont(-15, fwNormal)
	win.fontH = makeFont(-24, fwBold)
	win.fontBtn = makeFont(-17, fwBold)

	// Load the embedded application icon so it shows in the window's caption
	// bar (next to the close button), the taskbar and Alt+Tab. The window is
	// drawn with raw Win32 here (unlike the Gio GUI, which loads it itself), so
	// the class must reference the icon explicitly or Windows uses its default.
	hIcon, _, _ := procLoadImage.Call(hInst, iconResID, imageIcon, 0, 0, lrDefaultSize|lrShared)

	className := utf16("D2PSetupWindow")
	wc := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInst,
		hIcon:         hIcon,
		hbrBackground: win.hbrBg,
		lpszClassName: className,
		hIconSm:       hIcon,
	}
	hCursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512) // IDC_ARROW
	wc.hCursor = hCursor
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	const w, h = 420, 250
	// Center on the primary monitor's work area (excludes the taskbar) so the
	// window never appears under it. Fall back to full-screen metrics if the
	// query fails.
	var x, y int32
	var wa rect
	if r, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0); r != 0 {
		x = wa.left + (wa.right-wa.left-w)/2
		y = wa.top + (wa.bottom-wa.top-h)/2
	} else {
		cx, _, _ := procGetSystemMetrics.Call(smCxScreen)
		cy, _, _ := procGetSystemMetrics.Call(smCyScreen)
		x = (int32(cx) - w) / 2
		y = (int32(cy) - h) / 2
	}

	style := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox)
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16(config.DisplayName))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		0, 0, hInst, 0,
	)
	win.hwnd = hwnd

	win.createControls()

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	// If the install completed successfully, relaunch the installed GUI.
	win.mu.Lock()
	done := win.done
	win.mu.Unlock()
	if done {
		_ = installer.RelaunchInstalled()
	}
}

func (s *setupWindow) createControls() {
	// Owner-drawn primary "Install" button.
	s.hInstallBtn, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(utf16("BUTTON"))),
		uintptr(unsafe.Pointer(utf16("Установить"))),
		uintptr(wsChild|wsVisible|wsTabStop|bsOwnerDraw),
		30, 78, 360, 46,
		s.hwnd, idInstall, s.hInst, 0,
	)

	// Native checkboxes, recolored via WM_CTLCOLORSTATIC.
	s.hDesktopChk, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(utf16("BUTTON"))),
		uintptr(unsafe.Pointer(utf16("Ярлык на рабочем столе"))),
		uintptr(wsChild|wsVisible|wsTabStop|bsAutoCheckBox),
		34, 138, 340, 24,
		s.hwnd, idDesktop, s.hInst, 0,
	)
	s.hStartChk, _, _ = procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(utf16("BUTTON"))),
		uintptr(unsafe.Pointer(utf16("Ярлык в меню Пуск"))),
		uintptr(wsChild|wsVisible|wsTabStop|bsAutoCheckBox),
		34, 166, 340, 24,
		s.hwnd, idStartMnu, s.hInst, 0,
	)

	// Both checked by default.
	procSendMessage.Call(s.hDesktopChk, bmSetCheck, bstChecked, 0)
	procSendMessage.Call(s.hStartChk, bmSetCheck, bstChecked, 0)

	// Apply the UI font to checkboxes.
	procSendMessage.Call(s.hDesktopChk, wmSetFont, s.fontBase, 1)
	procSendMessage.Call(s.hStartChk, wmSetFont, s.fontBase, 1)
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		id := loword(wParam)
		notify := hiword(wParam)
		if id == idInstall && notify == bnClicked {
			win.onInstallClicked()
		}
		return 0

	case wmDrawItem:
		dis := (*drawItemStruct)(*(*unsafe.Pointer)(unsafe.Pointer(&lParam)))
		if dis.CtlID == idInstall {
			win.drawInstallButton(dis)
			return 1
		}
		return 0

	case wmCtlColorStatic, wmCtlColorBtn:
		hdc := wParam
		procSetBkMode.Call(hdc, transparent)
		procSetTextColor.Call(hdc, clrText)
		return win.hbrBg

	case wmPaint:
		win.onPaint()
		return 0

	case wmAppProgress:
		// Repaint just the bottom status/progress strip, erasing it first so
		// the new text/bar doesn't overlap the previous frame.
		win.invalidateStatus()
		return 0

	case wmTimer:
		if wParam == timerPulse {
			win.mu.Lock()
			win.pulse += 0.06
			if win.pulse > 1 {
				win.pulse -= 1
			}
			indeterminate := win.showBar && win.progress < 0
			win.mu.Unlock()
			if indeterminate {
				win.invalidateStatus()
			}
		}
		return 0

	case wmAppDone:
		// wParam: 1 = success, 0 = failure.
		procKillTimer.Call(hwnd, timerPulse)
		if wParam == 1 {
			win.mu.Lock()
			win.done = true
			win.mu.Unlock()
			procDestroyWindow.Call(hwnd)
		} else {
			// Install failed: leave install mode and restore the controls so
			// the user can retry.
			win.mu.Lock()
			win.busy = false
			win.installing = false
			win.mu.Unlock()
			procShowWindow.Call(win.hInstallBtn, swShow)
			procShowWindow.Call(win.hDesktopChk, swShow)
			procShowWindow.Call(win.hStartChk, swShow)
			procInvalidateRect.Call(hwnd, 0, 1)
			procUpdateWindow.Call(hwnd)
		}
		return 0

	case wmDestroy:
		if win.hbrBg != 0 {
			procDeleteObject.Call(win.hbrBg)
		}
		if win.hbrBg2 != 0 {
			procDeleteObject.Call(win.hbrBg2)
		}
		if win.hbrAccent != 0 {
			procDeleteObject.Call(win.hbrAccent)
		}
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

// drawInstallButton renders the owner-drawn accent button with rounded corners.
func (s *setupWindow) drawInstallButton(dis *drawItemStruct) {
	hdc := dis.hDC
	rc := dis.rcItem
	w := rc.right - rc.left
	h := rc.bottom - rc.top

	disabled := dis.itemState&odsDisabled != 0

	bg := clrAccent
	if disabled {
		bg = clrAccent2
	}

	// Rounded-rect fill using the DC brush/pen.
	dcBrushObj, _, _ := procGetStockObject.Call(dcBrush)
	dcPenObj, _, _ := procGetStockObject.Call(dcPen)
	oldBrush, _, _ := procSelectObject.Call(hdc, dcBrushObj)
	oldPen, _, _ := procSelectObject.Call(hdc, dcPenObj)
	procSetDCBrushColor.Call(hdc, bg)
	procSetDCPenColor.Call(hdc, bg)
	procRoundRect.Call(hdc, 0, 0, uintptr(w), uintptr(h), 16, 16)
	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)

	// Centered label.
	oldFont, _, _ := procSelectObject.Call(hdc, s.fontBtn)
	procSetBkMode.Call(hdc, transparent)
	procSetTextColor.Call(hdc, clrText)
	label := utf16("Установить")
	r := rc
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(label)), ^uintptr(0),
		uintptr(unsafe.Pointer(&r)), dtCenter|dtVcenter|dtSingleLine)
	procSelectObject.Call(hdc, oldFont)
}

// onPaint draws the dark-themed content. In normal mode it shows the title at
// the top (controls are drawn by Windows). In install mode every control is
// hidden and only a centered status text + progress bar are painted.
func (s *setupWindow) onPaint() {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(s.hwnd, uintptr(unsafe.Pointer(&ps)))

	var cr rect
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&cr)))

	// The window class background brush (clrBg) erases the invalidated region
	// before WM_PAINT, so we only draw foreground content here.
	procSetBkMode.Call(hdc, transparent)

	s.mu.Lock()
	installing := s.installing
	statusMsg := s.statusMsg
	statusColor := s.statusColor
	showBar := s.showBar
	progress := s.progress
	pulse := s.pulse
	s.mu.Unlock()

	if installing {
		s.paintInstalling(hdc, &cr, statusMsg, statusColor, showBar, progress, pulse)
		procEndPaint.Call(s.hwnd, uintptr(unsafe.Pointer(&ps)))
		return
	}

	// Normal mode: title at the top (the button/checkboxes are native windows).
	oldFont, _, _ := procSelectObject.Call(hdc, s.fontH)
	procSetTextColor.Call(hdc, clrText)
	titleRect := rect{left: 30, top: 24, right: cr.right - 30, bottom: 64}
	title := utf16(config.DisplayName)
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(title)), ^uintptr(0),
		uintptr(unsafe.Pointer(&titleRect)), dtLeft|dtSingleLine|dtVcenter)
	procSelectObject.Call(hdc, oldFont)

	// Any residual status line (e.g. an error) sits at the bottom.
	if statusMsg != "" {
		oldFont2, _, _ := procSelectObject.Call(hdc, s.fontBase)
		procSetTextColor.Call(hdc, statusColor)
		stRect := rect{left: 30, top: cr.bottom - 26, right: cr.right - 30, bottom: cr.bottom - 4}
		txt := utf16(statusMsg)
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(txt)), ^uintptr(0),
			uintptr(unsafe.Pointer(&stRect)), dtLeft|dtSingleLine|dtVcenter)
		procSelectObject.Call(hdc, oldFont2)
	}

	procEndPaint.Call(s.hwnd, uintptr(unsafe.Pointer(&ps)))
}

// paintInstalling renders the centered status text above a progress bar while
// the install runs (all controls hidden).
func (s *setupWindow) paintInstalling(hdc uintptr, cr *rect, statusMsg string, statusColor uintptr, showBar bool, progress, pulse float64) {
	cx := cr.right / 2
	cy := cr.bottom / 2

	// Centered status text just above the bar.
	if statusMsg != "" {
		oldFont, _, _ := procSelectObject.Call(hdc, s.fontBase)
		procSetTextColor.Call(hdc, statusColor)
		stRect := rect{left: 30, top: cy - 34, right: cr.right - 30, bottom: cy - 12}
		txt := utf16(statusMsg)
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(txt)), ^uintptr(0),
			uintptr(unsafe.Pointer(&stRect)), dtCenter|dtSingleLine|dtVcenter)
		procSelectObject.Call(hdc, oldFont)
	}

	if !showBar {
		return
	}

	const barW = 300
	const barH = 8
	left := cx - barW/2
	right := cx + barW/2
	top := cy
	track := rect{left: left, top: top, right: right, bottom: top + barH}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&track)), s.hbrBg2)

	trackW := track.right - track.left
	if progress >= 0 {
		fillW := int32(float64(trackW) * progress)
		fill := rect{left: track.left, top: track.top, right: track.left + fillW, bottom: track.bottom}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&fill)), s.hbrAccent)
	} else {
		segW := trackW / 3
		start := track.left + int32(float64(trackW)*pulse)
		end := start + segW
		if end > track.right {
			end = track.right
		}
		seg := rect{left: start, top: track.top, right: end, bottom: track.bottom}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&seg)), s.hbrAccent)
	}
}

func (s *setupWindow) setStatus(msg string, color uintptr) {
	s.mu.Lock()
	s.statusMsg = msg
	s.statusColor = color
	s.mu.Unlock()
	procPostMessage.Call(s.hwnd, wmAppProgress, 0, 0)
}

// invalidateStatus marks the status/progress region dirty with erase=TRUE (so
// the previous frame is cleared) and forces an immediate repaint of just that
// region. The region differs between normal mode (bottom strip) and install
// mode (centered band).
func (s *setupWindow) invalidateStatus() {
	var cr rect
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&cr)))

	s.mu.Lock()
	installing := s.installing
	s.mu.Unlock()

	var region rect
	if installing {
		cy := cr.bottom / 2
		region = rect{left: 0, top: cy - 40, right: cr.right, bottom: cy + 20}
	} else {
		region = rect{left: 0, top: cr.bottom - 52, right: cr.right, bottom: cr.bottom}
	}
	procInvalidateRect.Call(s.hwnd, uintptr(unsafe.Pointer(&region)), 1)
	procUpdateWindow.Call(s.hwnd)
}

func (s *setupWindow) onInstallClicked() {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.installing = true
	s.showBar = true
	s.progress = -1
	s.mu.Unlock()

	desktop := isChecked(s.hDesktopChk)
	startMenu := isChecked(s.hStartChk)

	// Enter install mode: hide every control, leaving only the centered
	// status text and progress bar drawn by onPaint.
	procShowWindow.Call(s.hInstallBtn, swHide)
	procShowWindow.Call(s.hDesktopChk, swHide)
	procShowWindow.Call(s.hStartChk, swHide)
	procInvalidateRect.Call(s.hwnd, 0, 1) // full repaint to clear the title/old layout
	procUpdateWindow.Call(s.hwnd)

	// Animate the indeterminate bar until the first real progress arrives.
	procSetTimer.Call(s.hwnd, timerPulse, pulsePeriodMs, 0)
	s.setStatus("Скачивание…", clrText)

	go func() {
		onProgress := func(downloaded, total int64) {
			s.mu.Lock()
			if total > 0 {
				s.progress = float64(downloaded) / float64(total)
			} else {
				s.progress = -1
			}
			s.mu.Unlock()
			procPostMessage.Call(s.hwnd, wmAppProgress, 0, 0)
		}

		err := runInstall(desktop, startMenu, onProgress)
		if err != nil {
			// installing/busy are cleared by the wmAppDone handler, which also
			// restores the controls; here we only set the failure message/bar.
			s.mu.Lock()
			s.showBar = false
			s.statusMsg = "Ошибка: " + err.Error()
			s.statusColor = clrRed
			s.mu.Unlock()
			procPostMessage.Call(s.hwnd, wmAppDone, 0, 0)
			return
		}

		s.setStatus("Установлено! Запуск…", clrGreen)
		// Brief pause so the success message is visible.
		time.Sleep(700 * time.Millisecond)
		procPostMessage.Call(s.hwnd, wmAppDone, 1, 0)
	}()
}

func isChecked(hwndCtl uintptr) bool {
	r, _, _ := procSendMessage.Call(hwndCtl, bmGetCheck, 0, 0)
	return r == bstChecked
}
