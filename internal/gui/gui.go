package gui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
	"time"
	"unsafe"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/sys/windows"

	"discord-szx/internal/config"
	"discord-szx/internal/deploy"
	"discord-szx/internal/discord"
	"discord-szx/internal/proxy"
)

var (
	user32               = windows.NewLazyDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

const (
	smCXScreen  = 0
	smCYScreen  = 1
	swpNoSize   = 0x0001
	swpNoZOrder = 0x0004
)

func centerWindow(hwnd uintptr) {
	screenW, _, _ := procGetSystemMetrics.Call(smCXScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCYScreen)

	var rect windows.Rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))

	winW := rect.Right - rect.Left
	winH := rect.Bottom - rect.Top
	x := (int32(screenW) - winW) / 2
	y := (int32(screenH) - winH) / 2

	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder)
}

var (
	colBg       = color.NRGBA{R: 0x1E, G: 0x1E, B: 0x2E, A: 0xFF}
	colBg2      = color.NRGBA{R: 0x25, G: 0x25, B: 0x35, A: 0xFF}
	colAccent   = color.NRGBA{R: 0xA9, G: 0x7B, B: 0xFF, A: 0xFF}
	colAccent2  = color.NRGBA{R: 0x7B, G: 0x5E, B: 0xB5, A: 0xFF}
	colText     = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colTextDim  = color.NRGBA{R: 0xA0, G: 0xA0, B: 0xB0, A: 0xFF}
	colGreen    = color.NRGBA{R: 0x4A, G: 0xDE, B: 0x80, A: 0xFF}
	colRed      = color.NRGBA{R: 0xF8, G: 0x71, B: 0x71, A: 0xFF}
	colDisabled = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x4A, A: 0xFF}
	colDivider  = color.NRGBA{R: 0x3A, G: 0x3A, B: 0x4A, A: 0xFF}
)

type UI struct {
	cfg      *config.Config
	theme    *material.Theme
	window   *app.Window
	stopChan chan struct{}

	// mu guards all mutable state below, which is written from worker
	// goroutines (detection, install, uninstall) and read by the render loop.
	mu             sync.Mutex
	install        *discord.DiscordInstall
	proxyInfo      *proxy.ProxyInfo
	discordErr     error
	proxyErr       error
	installed      bool
	detecting      bool // initial detection in progress
	busy           bool // install/uninstall in progress
	discordRunning bool // updated by the background watcher

	btnInstall   widget.Clickable
	btnUninstall widget.Clickable
	chkForce     widget.Bool

	statusMsg   string
	statusColor color.NRGBA
	statusTime  time.Time
	hwnd        uintptr
	centered    bool
}

func Run() {
	cfg := config.Default()
	ui := &UI{
		cfg:       cfg,
		detecting: true,
		stopChan:  make(chan struct{}),
	}

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Regular()))
	th.Palette = material.Palette{
		Bg:         colBg,
		Fg:         colText,
		ContrastBg: colAccent,
		ContrastFg: colText,
	}
	ui.theme = th

	w := new(app.Window)
	w.Option(
		app.Size(unit.Dp(420), unit.Dp(320)),
		app.Title(config.Title()),
		app.MinSize(unit.Dp(420), unit.Dp(320)),
		app.MaxSize(unit.Dp(420), unit.Dp(320)),
	)
	ui.window = w

	// Detect Discord and proxy asynchronously so the window appears
	// immediately instead of freezing for up to several seconds.
	go ui.detect()

	// Continuously watch whether Discord is running so the "force" control
	// surfaces automatically (and hides again when Discord is closed).
	go ui.watchDiscord()

	go func() {
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				ui.layout(gtx)
				e.Frame(gtx.Ops)
				if ui.hwnd != 0 && !ui.centered {
					ui.centered = true
					hwnd := ui.hwnd
					w.Run(func() { centerWindow(hwnd) })
				}
			case app.Win32ViewEvent:
				if e.Valid() && ui.hwnd == 0 {
					ui.hwnd = e.HWND
				}
			case app.DestroyEvent:
				close(ui.stopChan)
				os.Exit(0)
			}
		}
	}()
	app.Main()
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, colBg)

	ui.mu.Lock()
	canInstall := ui.install != nil && ui.proxyInfo != nil && !ui.busy && !ui.detecting
	canUninstall := ui.install != nil && ui.installed && !ui.busy && !ui.detecting
	statusMsg := ui.statusMsg
	statusColor := ui.statusColor
	statusTime := ui.statusTime
	ui.mu.Unlock()

	force := ui.chkForce.Value
	if ui.btnInstall.Clicked(gtx) && canInstall {
		go ui.doInstall(force)
	}
	if ui.btnUninstall.Clicked(gtx) && canUninstall {
		go ui.doUninstall(force)
	}

	if statusMsg != "" && statusColor == colGreen && !statusTime.IsZero() && time.Since(statusTime) > 5*time.Second {
		ui.mu.Lock()
		ui.statusMsg = ""
		ui.statusTime = time.Time{}
		ui.mu.Unlock()
	} else if statusMsg != "" && statusColor == colGreen && !statusTime.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: statusTime.Add(5 * time.Second)})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(ui.titleRow),
		layout.Rigid(ui.divider),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(4)}.Layout(gtx, ui.statusCards)
		}),
		layout.Rigid(ui.detailText),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, ui.buttons)
		}),
		layout.Rigid(ui.forceRow),
		layout.Rigid(ui.statusBanner),
		layout.Flexed(1, ui.footer),
	)
}

func (ui *UI) titleRow(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Left: unit.Dp(20), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.H6(ui.theme, config.AppName)
		lbl.Color = colText
		return lbl.Layout(gtx)
	})
}

func (ui *UI) divider(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		paint.FillShape(gtx.Ops, colDivider, clip.Rect(image.Rect(0, 0, w, 1)).Op())
		return layout.Dimensions{Size: image.Pt(w, 1)}
	})
}

func (ui *UI) statusCards(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	discordOK := ui.discordErr == nil && ui.install != nil
	proxyOK := ui.proxyErr == nil && ui.proxyInfo != nil
	installed := ui.installed
	discordVal := ui.discordStatusLocked()
	proxyVal := ui.proxyStatusLocked()
	installVal := ui.installStatusLocked()
	ui.mu.Unlock()

	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gap := gtx.Dp(14)
		cardW := (gtx.Constraints.Max.X - gap*2) / 3
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "DISCORD", discordVal, discordOK)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "ПРОКСИ", proxyVal, proxyOK)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "СТАТУС", installVal, installed)
			}),
		)
	})
}

func (ui *UI) statusCard(gtx layout.Context, label, value string, ok bool) layout.Dimensions {
	h := gtx.Dp(48)
	r := gtx.Dp(8)
	w := gtx.Constraints.Min.X

	paint.FillShape(gtx.Ops, colBg2, clip.UniformRRect(image.Rect(0, 0, w, h), r).Op(gtx.Ops))

	dotR := gtx.Dp(5)
	dotX := w - dotR*2 - gtx.Dp(12)
	dotY := (h - dotR*2) / 2
	dotCol := colRed
	if ok {
		dotCol = colGreen
	}
	paint.FillShape(gtx.Ops, dotCol, clip.Ellipse(image.Rect(dotX, dotY, dotX+dotR*2, dotY+dotR*2)).Op(gtx.Ops))

	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Left: unit.Dp(12), Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(ui.theme, label)
		lbl.Color = colTextDim
		lbl.Font.Weight = font.Medium
		return lbl.Layout(gtx)
	})
	call := macro.Stop()
	call.Add(gtx.Ops)

	macro2 := op.Record(gtx.Ops)
	dims2 := layout.Inset{Left: unit.Dp(12), Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Subtitle2(ui.theme, value)
		lbl.Color = colText
		return lbl.Layout(gtx)
	})
	call2 := macro2.Stop()
	call2.Add(gtx.Ops)

	_ = dims
	_ = dims2

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (ui *UI) detailText(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	var txt string
	switch {
	case ui.detecting:
		txt = "Поиск Discord и прокси..."
	case ui.install != nil:
		txt = fmt.Sprintf("Найден: %s (%s)", ui.install.Channel, ui.install.Version)
	default:
		txt = "Discord не найден"
	}
	ui.mu.Unlock()

	return layout.Inset{Left: unit.Dp(20), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(ui.theme, txt)
		lbl.Color = colTextDim
		return lbl.Layout(gtx)
	})
}

func (ui *UI) buttons(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	hasInstall := ui.install != nil
	hasProxy := ui.proxyInfo != nil
	installed := ui.installed
	blocked := ui.busy || ui.detecting
	ui.mu.Unlock()

	canInstall := hasInstall && hasProxy && !blocked
	canUninstall := hasInstall && installed && !blocked

	installLabel := "Установить"
	if installed {
		installLabel = "Переустановить"
	}
	if blocked {
		installLabel = "Подождите..."
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(ui.theme, &ui.btnInstall, installLabel)
				btn.CornerRadius = 8
				if canInstall {
					btn.Background = colAccent
					btn.Color = colText
				} else {
					btn.Background = colDisabled
					btn.Color = colTextDim
				}
				return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, btn.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				btn := material.Button(ui.theme, &ui.btnUninstall, "Удалить")
				btn.CornerRadius = 8
				if canUninstall {
					btn.Background = colAccent2
					btn.Color = colText
				} else {
					btn.Background = colDisabled
					btn.Color = colTextDim
				}
				return btn.Layout(gtx)
			}),
		)
	})
}

func (ui *UI) forceRow(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	running := ui.discordRunning
	ui.mu.Unlock()

	// The force control is only relevant while Discord is running and is
	// holding file locks; keep it hidden otherwise to reduce clutter.
	if !running {
		return layout.Dimensions{}
	}

	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cb := material.CheckBox(ui.theme, &ui.chkForce, "Discord запущен — установить принудительно")
			cb.Color = colRed
			cb.IconColor = colRed
			cb.TextSize = unit.Sp(12)
			return cb.Layout(gtx)
		})
	})
}

func (ui *UI) statusBanner(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	msg := ui.statusMsg
	col := ui.statusColor
	ui.mu.Unlock()
	if msg == "" {
		return layout.Dimensions{}
	}
	return ui.statusBannerDraw(gtx, msg, col)
}

func (ui *UI) statusBannerDraw(gtx layout.Context, msg string, col color.NRGBA) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(28)
		w := gtx.Constraints.Max.X
		r := gtx.Dp(6)

		paint.FillShape(gtx.Ops, colBg2, clip.UniformRRect(image.Rect(0, 0, w, h), r).Op(gtx.Ops))

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(ui.theme, msg)
			lbl.Color = col
			return lbl.Layout(gtx)
		})
	})
}

func (ui *UI) footer(gtx layout.Context) layout.Dimensions {
	return layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(8), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(ui.theme, "by siNeZix")
			lbl.Color = colTextDim
			return lbl.Layout(gtx)
		})
	})
}

// *Locked helpers must be called with ui.mu held.

func (ui *UI) discordStatusLocked() string {
	if ui.discordErr != nil || ui.install == nil {
		return "Нет"
	}
	return "ОК"
}

func (ui *UI) proxyStatusLocked() string {
	if ui.proxyErr != nil || ui.proxyInfo == nil {
		return "Нет"
	}
	return "ОК"
}

func (ui *UI) installStatusLocked() string {
	if ui.installed {
		return "Активно"
	}
	return "Неактивно"
}

// detect runs the initial Discord + proxy discovery off the render thread.
func (ui *UI) detect() {
	install, derr := discord.FindPrimaryDiscord(ui.cfg)
	proxyInfo, perr := proxy.DetectBestProxy("127.0.0.1", ui.cfg.ProxyPorts)

	ui.mu.Lock()
	if derr != nil {
		ui.discordErr = derr
		ui.install = nil
	} else {
		ui.install = install
		ui.installed = deploy.IsInstalled(install, ui.cfg)
	}
	if perr != nil {
		ui.proxyErr = perr
		ui.proxyInfo = nil
	} else {
		ui.proxyInfo = proxyInfo
	}
	ui.detecting = false
	ui.mu.Unlock()
	ui.window.Invalidate()
}

// watchDiscord polls whether the detected Discord channel is running and
// repaints the UI only when the state changes, so the force control can
// appear/disappear without the user clicking anything.
func (ui *UI) watchDiscord() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ui.stopChan:
			return
		case <-ticker.C:
			ui.mu.Lock()
			install := ui.install
			prev := ui.discordRunning
			ui.mu.Unlock()
			if install == nil {
				if prev {
					ui.mu.Lock()
					ui.discordRunning = false
					ui.mu.Unlock()
					ui.window.Invalidate()
				}
				continue
			}
			running := deploy.IsDiscordRunning(install.Channel)
			if running != prev {
				ui.mu.Lock()
				ui.discordRunning = running
				ui.mu.Unlock()
				ui.window.Invalidate()
			}
		}
	}
}

func (ui *UI) doInstall(force bool) {
	ui.mu.Lock()
	if ui.install == nil || ui.proxyInfo == nil || ui.busy {
		ui.mu.Unlock()
		return
	}
	ui.busy = true
	install := ui.install
	proxyInfo := ui.proxyInfo
	reinstall := ui.installed
	ui.mu.Unlock()
	ui.window.Invalidate()

	d := deploy.New(ui.cfg, false, force)
	if err := d.Deploy(install, proxyInfo); err != nil {
		ui.finishStatus(fmt.Sprintf("Ошибка установки: %v", err), false, false)
		return
	}
	if err := d.Verify(install); err != nil {
		ui.finishStatus(fmt.Sprintf("Ошибка проверки: %v", err), false, false)
		return
	}
	msg := "Прокси установлен! Перезапустите Discord."
	if reinstall {
		msg = "Прокси переустановлен! Перезапустите Discord."
	}
	ui.finishStatus(msg, true, true)
}

func (ui *UI) doUninstall(force bool) {
	ui.mu.Lock()
	if ui.install == nil || ui.busy {
		ui.mu.Unlock()
		return
	}
	ui.busy = true
	install := ui.install
	ui.mu.Unlock()
	ui.window.Invalidate()

	d := deploy.New(ui.cfg, false, force)
	if err := d.Uninstall(install); err != nil {
		ui.finishStatus(fmt.Sprintf("Ошибка удаления: %v", err), false, false)
		return
	}
	ui.finishStatusInstalled(false, "Прокси удалён! Перезапустите Discord.", true)
}

// finishStatus clears busy, sets the banner and optionally re-detects the
// installed state from disk so the UI reflects reality after the operation.
func (ui *UI) finishStatus(msg string, ok, reReadInstalled bool) {
	ui.mu.Lock()
	ui.busy = false
	if reReadInstalled && ui.install != nil {
		ui.installed = deploy.IsInstalled(ui.install, ui.cfg)
	}
	ui.setStatusLocked(msg, ok)
	ui.mu.Unlock()
	ui.window.Invalidate()
}

func (ui *UI) finishStatusInstalled(installed bool, msg string, ok bool) {
	ui.mu.Lock()
	ui.busy = false
	ui.installed = installed
	ui.setStatusLocked(msg, ok)
	ui.mu.Unlock()
	ui.window.Invalidate()
}

func (ui *UI) setStatusLocked(msg string, ok bool) {
	ui.statusMsg = msg
	if ok {
		ui.statusColor = colGreen
	} else {
		ui.statusColor = colRed
	}
	ui.statusTime = time.Now()
}
