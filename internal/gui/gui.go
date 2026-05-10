package gui

import (
	"fmt"
	"image"
	"image/color"
	"os"
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
	user32             = windows.NewLazyDLL("user32.dll")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

const (
	smCXScreen   = 0
	smCYScreen   = 1
	swpNoSize    = 0x0001
	swpNoZOrder  = 0x0004
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
	cfg    *config.Config
	theme  *material.Theme
	window *app.Window

	install    *discord.DiscordInstall
	proxyInfo  *proxy.ProxyInfo
	discordErr error
	proxyErr   error
	installed  bool

	btnInstall   widget.Clickable
	btnUninstall widget.Clickable

	statusMsg   string
	statusColor color.NRGBA
	statusTime  time.Time
	hwnd        uintptr
	centered    bool
}

func Run() {
	cfg := config.Default()
	ui := &UI{cfg: cfg}

	install, err := discord.FindPrimaryDiscord(cfg)
	if err != nil {
		ui.discordErr = err
	} else {
		ui.install = install
		ui.installed = deploy.IsInstalled(install, cfg)
	}

	proxyInfo, err := proxy.DetectBestProxy("127.0.0.1", cfg.ProxyPorts)
	if err != nil {
		ui.proxyErr = err
	} else {
		ui.proxyInfo = proxyInfo
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
		app.Title("discord-szx"),
		app.MinSize(unit.Dp(420), unit.Dp(320)),
		app.MaxSize(unit.Dp(420), unit.Dp(320)),
	)
	ui.window = w

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
				os.Exit(0)
			}
		}
	}()
	app.Main()
}

func (ui *UI) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, colBg)

	if ui.btnInstall.Clicked(gtx) && ui.install != nil && ui.proxyInfo != nil {
		ui.doInstall()
	}
	if ui.btnUninstall.Clicked(gtx) && ui.install != nil && ui.installed {
		ui.doUninstall()
	}

	if ui.statusMsg != "" && ui.statusColor == colGreen && !ui.statusTime.IsZero() && time.Since(ui.statusTime) > 5*time.Second {
		ui.statusMsg = ""
		ui.statusTime = time.Time{}
	}
	if ui.statusMsg != "" && ui.statusColor == colGreen && !ui.statusTime.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: ui.statusTime.Add(5 * time.Second)})
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
		layout.Rigid(ui.statusBanner),
		layout.Flexed(1, ui.footer),
	)
}

func (ui *UI) titleRow(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Left: unit.Dp(20), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.H6(ui.theme, "discord-szx")
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
	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gap := gtx.Dp(14)
		cardW := (gtx.Constraints.Max.X - gap*2) / 3
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "DISCORD", ui.discordStatus(), ui.discordErr == nil)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "ПРОКСИ", ui.proxyStatus(), ui.proxyErr == nil)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return ui.statusCard(gtx, "СТАТУС", ui.installStatus(), ui.installed)
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
	return layout.Inset{Left: unit.Dp(20), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		txt := "Discord не найден"
		if ui.install != nil {
			txt = fmt.Sprintf("Найден: %s (%s)", ui.install.Channel, ui.install.Version)
		}
		lbl := material.Caption(ui.theme, txt)
		lbl.Color = colTextDim
		return lbl.Layout(gtx)
	})
}

func (ui *UI) buttons(gtx layout.Context) layout.Dimensions {
	canInstall := ui.install != nil && ui.proxyInfo != nil
	canUninstall := ui.install != nil && ui.installed

	installLabel := "Установить"
	if ui.installed {
		installLabel = "Переустановить"
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

func (ui *UI) statusBanner(gtx layout.Context) layout.Dimensions {
	if ui.statusMsg == "" {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(28)
		w := gtx.Constraints.Max.X
		r := gtx.Dp(6)

		paint.FillShape(gtx.Ops, colBg2, clip.UniformRRect(image.Rect(0, 0, w, h), r).Op(gtx.Ops))

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(ui.theme, ui.statusMsg)
			lbl.Color = ui.statusColor
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

func (ui *UI) discordStatus() string {
	if ui.discordErr != nil {
		return "Нет"
	}
	return "ОК"
}

func (ui *UI) proxyStatus() string {
	if ui.proxyErr != nil {
		return "Нет"
	}
	return "ОК"
}

func (ui *UI) installStatus() string {
	if ui.installed {
		return "Активно"
	}
	return "Неактивно"
}

func (ui *UI) doInstall() {
	if ui.install == nil || ui.proxyInfo == nil {
		return
	}
	reinstall := ui.installed
	d := deploy.New(ui.cfg, false, false)
	if err := d.Deploy(ui.install, ui.proxyInfo); err != nil {
		ui.showStatus(fmt.Sprintf("Ошибка установки: %v", err), false)
		return
	}
	if err := d.Verify(ui.install); err != nil {
		ui.showStatus(fmt.Sprintf("Ошибка проверки: %v", err), false)
		return
	}
	ui.installed = true
	if reinstall {
		ui.showStatus("Прокси переустановлен! Перезапустите Discord.", true)
	} else {
		ui.showStatus("Прокси установлен! Перезапустите Discord.", true)
	}
}

func (ui *UI) doUninstall() {
	if ui.install == nil {
		return
	}
	d := deploy.New(ui.cfg, false, false)
	if err := d.Uninstall(ui.install); err != nil {
		ui.showStatus(fmt.Sprintf("Ошибка удаления: %v", err), false)
		return
	}
	ui.installed = false
	ui.showStatus("Прокси удалён! Перезапустите Discord.", true)
}

func (ui *UI) showStatus(msg string, ok bool) {
	ui.statusMsg = msg
	if ok {
		ui.statusColor = colGreen
	} else {
		ui.statusColor = colRed
	}
	ui.statusTime = time.Now()
	ui.window.Invalidate()
}
