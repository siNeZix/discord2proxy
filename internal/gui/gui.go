package gui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"discord-szx/internal/assets"
	"discord-szx/internal/config"
	"discord-szx/internal/deploy"
	"discord-szx/internal/discord"
	"discord-szx/internal/installer"
	"discord-szx/internal/proxy"
	"discord-szx/internal/update"
)

type uiPhase int

const (
	phaseMain uiPhase = iota
	phasePrompt
)

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
	logoOp   paint.ImageOp
	tgOp     paint.ImageOp
	ghOp     paint.ImageOp

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

	// Update state, written by checkUpdate/doUpdate and read by the renderer.
	updateRel  *update.Release
	updateBusy bool

	btnInstall   widget.Clickable
	btnUninstall widget.Clickable
	btnUpdate    widget.Clickable
	btnTelegram  widget.Clickable
	btnGithub    widget.Clickable
	chkForce     widget.Bool
	chkDesktop   widget.Bool
	chkStartMenu widget.Bool

	phase            uiPhase
	btnPromptInstall widget.Clickable
	btnPromptNotNow  widget.Clickable

	statusMsg   string
	statusColor color.NRGBA
	statusTime  time.Time
	busyMsg     string
	busyStart   time.Time
}

func Run(noRelaunch bool) {
	if !noRelaunch && installer.IsInstalled() && !installer.IsRunningFromInstallDir() {
		if err := installer.RelaunchInstalled(); err == nil {
			// Installed copy launched successfully; hand off and exit so we
			// don't run a second instance from the portable location.
			os.Exit(0)
		}
		// On failure fall through and keep running from the current location.
	}

	startPhase := phaseMain
	if !installer.IsInstalled() && !installer.IsRunningFromInstallDir() {
		startPhase = phasePrompt
	}

	var logoOp paint.ImageOp
	if img, _, err := image.Decode(bytes.NewReader(assets.LogoPNG)); err == nil {
		logoOp = paint.NewImageOp(img)
	}

	var tgOp, ghOp paint.ImageOp
	if img, _, err := image.Decode(bytes.NewReader(assets.TelegramPNG)); err == nil {
		tgOp = paint.NewImageOp(img)
	}
	if img, _, err := image.Decode(bytes.NewReader(assets.GithubPNG)); err == nil {
		ghOp = paint.NewImageOp(img)
	}

	cfg := config.Default()
	ui := &UI{
		cfg:       cfg,
		detecting: true,
		stopChan:  make(chan struct{}),
		phase:     startPhase,
		logoOp:    logoOp,
		tgOp:      tgOp,
		ghOp:      ghOp,
	}
	ui.chkDesktop.Value = true
	ui.chkStartMenu.Value = true

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

	// Check for a newer release in the background so the update button can
	// appear without blocking startup.
	go ui.checkUpdate()

	go func() {
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				ui.layout(gtx)
				e.Frame(gtx.Ops)
				// Window centering is handled natively by the patched Gio
				// (third_party/gioui.org) at first windowed placement, so the
				// window is born centered with no startup jump.
			case app.DestroyEvent:
				// Don't tear the process down while a self-update is mid-flight;
				// update.Apply replaces the binary and update.Restart relaunches
				// us, so an early os.Exit here would abort the upgrade.
				ui.mu.Lock()
				updating := ui.updateBusy
				ui.mu.Unlock()
				if updating {
					continue
				}
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
	phase := ui.phase
	ui.mu.Unlock()

	if phase == phasePrompt {
		return ui.promptLayout(gtx, phase)
	}

	ui.mu.Lock()
	canInstall := ui.install != nil && ui.proxyInfo != nil && !ui.busy && !ui.detecting
	canUninstall := ui.install != nil && ui.installed && !ui.busy && !ui.detecting
	statusMsg := ui.statusMsg
	statusColor := ui.statusColor
	statusTime := ui.statusTime
	busy := ui.busy
	busyStart := ui.busyStart
	canUpdate := ui.updateRel != nil && !ui.updateBusy
	ui.mu.Unlock()

	force := ui.chkForce.Value
	if ui.btnInstall.Clicked(gtx) && canInstall {
		go ui.doInstall(force)
	}
	if ui.btnUninstall.Clicked(gtx) && canUninstall {
		go ui.doUninstall(force)
	}
	if ui.btnUpdate.Clicked(gtx) && canUpdate {
		go ui.doUpdate()
	}
	if ui.btnTelegram.Clicked(gtx) {
		openURL("https://t.me/siNeZix")
	}
	if ui.btnGithub.Clicked(gtx) {
		openURL(fmt.Sprintf("https://github.com/%s/%s", config.RepoOwner, config.RepoName))
	}

	if busy && !busyStart.IsZero() {
		since := time.Since(busyStart)
		if since < time.Second {
			gtx.Execute(op.InvalidateCmd{At: busyStart.Add(time.Second)})
		}
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
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(380))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(380))
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, ui.buttons)
					}),
					layout.Rigid(ui.forceRow),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.footer(gtx, phase)
		}),
	)
}

func (ui *UI) titleRow(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(14), Left: unit.Dp(20), Right: unit.Dp(20), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ui.logoOp.Size().X == 0 {
					return layout.Dimensions{}
				}
				img := widget.Image{
					Src: ui.logoOp,
					Fit: widget.Contain,
				}
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(28))
				gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(28))
				gtx.Constraints.Min = gtx.Constraints.Max
				return img.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if ui.logoOp.Size().X == 0 {
					return layout.Dimensions{}
				}
				return layout.Spacer{Width: unit.Dp(10)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.H6(ui.theme, config.DisplayName)
				lbl.Color = colText
				return lbl.Layout(gtx)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(ui.updateButton),
		)
	})
}

// updateButton renders the "⚡ Обновить vX.Y.Z" pill in the top-right corner.
// It is hidden entirely when no newer release is available.
func (ui *UI) updateButton(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	rel := ui.updateRel
	busy := ui.updateBusy
	ui.mu.Unlock()

	if rel == nil {
		return layout.Dimensions{}
	}

	label := "Обновить v" + rel.Version
	btn := material.Button(ui.theme, &ui.btnUpdate, label)
	btn.CornerRadius = 8
	btn.TextSize = unit.Sp(12)
	btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
	if busy {
		btn.Background = colDisabled
		btn.Color = colTextDim
		gtx = gtx.Disabled()
	} else {
		btn.Background = colAccent
		btn.Color = colText
	}
	return btn.Layout(gtx)
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
	layout.Inset{Left: unit.Dp(12), Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(ui.theme, label)
		lbl.Color = colTextDim
		lbl.Font.Weight = font.Medium
		return lbl.Layout(gtx)
	})
	call := macro.Stop()
	call.Add(gtx.Ops)

	macro2 := op.Record(gtx.Ops)
	layout.Inset{Left: unit.Dp(12), Top: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Subtitle2(ui.theme, value)
		lbl.Color = colText
		return lbl.Layout(gtx)
	})
	call2 := macro2.Stop()
	call2.Add(gtx.Ops)

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (ui *UI) detailText(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	detecting := ui.detecting
	install := ui.install
	proxyInfo := ui.proxyInfo
	ui.mu.Unlock()

	caption := func(txt string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(ui.theme, txt)
			lbl.Color = colTextDim
			return lbl.Layout(gtx)
		}
	}

	if detecting {
		return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20), Top: unit.Dp(4)}.Layout(gtx, caption("Поиск Discord и прокси..."))
	}

	var discordLine, proxyLine string
	switch {
	case install != nil:
		discordLine = fmt.Sprintf("%s · %s", install.Channel, install.Version)
	default:
		discordLine = "Discord не найден"
	}

	if proxyInfo != nil {
		proxyLine = fmt.Sprintf("socks5://%s:%d", proxyInfo.Host, proxyInfo.Port)
	} else {
		proxyLine = "прокси не найден"
	}

	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20), Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gap := gtx.Dp(14)
		cardW := (gtx.Constraints.Max.X - gap*2) / 3

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = cardW
				gtx.Constraints.Min.X = cardW
				return caption(discordLine)(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return caption(proxyLine)(gtx)
			}),
		)
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
				// Suppress hover/press events so a non-actionable
				// button is visibly and functionally inert.
				gtx = gtx.Disabled()
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
				// Nothing to uninstall -> button is inert.
				gtx = gtx.Disabled()
			}
			return btn.Layout(gtx)
		}),
	)
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

	// Update the bool widget first so the value is fresh when calculating the label text
	ui.chkForce.Update(gtx)

	// The checkbox label switches to a warning while force is enabled, and
	// reverts to the original prompt when unchecked.
	label := "Discord запущен — установить принудительно"
	if ui.chkForce.Value {
		label = "Discord будет закрыт во время установки"
	}

	return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cb := material.CheckBox(ui.theme, &ui.chkForce, label)
			cb.Color = colRed
			cb.IconColor = colRed
			cb.Size = unit.Dp(19)
			cb.TextSize = unit.Sp(11.5)
			return cb.Layout(gtx)
		})
	})
}

func (ui *UI) statusBannerDraw(gtx layout.Context, msg string, col color.NRGBA) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		r := gtx.Dp(6)

		// Record the child dimensions to draw the background dynamically
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top:    unit.Dp(6),
			Bottom: unit.Dp(6),
			Left:   unit.Dp(10),
			Right:  unit.Dp(10),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(ui.theme, msg)
				lbl.Color = col
				lbl.TextSize = unit.Sp(11)
				return lbl.Layout(gtx)
			})
		})
		call := macro.Stop()

		// Draw background of matching size
		paint.FillShape(gtx.Ops, colBg2, clip.UniformRRect(image.Rect(0, 0, dims.Size.X, dims.Size.Y), r).Op(gtx.Ops))
		call.Add(gtx.Ops)

		return dims
	})
}

func (ui *UI) footer(gtx layout.Context, phase uiPhase) layout.Dimensions {
	ui.mu.Lock()
	msg := ui.statusMsg
	col := ui.statusColor
	busy := ui.busy
	busyMsg := ui.busyMsg
	busyStart := ui.busyStart
	ui.mu.Unlock()

	// If busy and more than 1 second has passed, show process status instead
	if busy && !busyStart.IsZero() && time.Since(busyStart) >= time.Second {
		msg = busyMsg
		col = colTextDim
	}

	return layout.S.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if msg == "" {
					return layout.Spacer{}.Layout(gtx)
				}
				return layout.Inset{Left: unit.Dp(20), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(ui.theme, msg)
					lbl.Color = col
					lbl.TextSize = unit.Sp(11)
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(8), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if phase == phasePrompt {
								return layout.Dimensions{}
							}
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return ui.linkIcon(gtx, &ui.btnTelegram, ui.tgOp)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return ui.linkIcon(gtx, &ui.btnGithub, ui.ghOp)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if phase == phasePrompt {
								return layout.Dimensions{}
							}
							return layout.Spacer{Height: unit.Dp(4)}.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(ui.theme, config.VersionTag()+" · by siNeZix")
							lbl.Color = colTextDim
							return lbl.Layout(gtx)
						}),
					)
				})
			}),
		)
	})
}

// linkIcon renders a small (16dp) clickable icon that shows a hand cursor on
// hover. Used for the Telegram/GitHub links in the footer.
func (ui *UI) linkIcon(gtx layout.Context, btn *widget.Clickable, op paint.ImageOp) layout.Dimensions {
	if op.Size().X == 0 {
		return layout.Dimensions{}
	}
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		sz := gtx.Dp(unit.Dp(16))
		gtx.Constraints.Min = image.Pt(sz, sz)
		gtx.Constraints.Max = image.Pt(sz, sz)
		pointer.CursorPointer.Add(gtx.Ops)
		img := widget.Image{Src: op, Fit: widget.Contain}
		return img.Layout(gtx)
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
			ui.mu.Unlock()

			running := install != nil && deploy.IsDiscordRunning(install.Channel)
			ui.setDiscordRunning(running)
		}
	}
}

// setDiscordRunning stores the running flag and repaints only when it changes,
// so the force control appears/disappears without spurious frames.
func (ui *UI) setDiscordRunning(running bool) {
	ui.mu.Lock()
	changed := ui.discordRunning != running
	ui.discordRunning = running
	ui.mu.Unlock()
	if changed {
		ui.window.Invalidate()
	}
}

// checkUpdate queries GitHub Releases once on startup and, if a newer build
// is available, stores it so the update button appears.
func (ui *UI) checkUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rel, newer, err := update.CheckLatest(ctx)
	if err != nil || !newer {
		return
	}

	ui.mu.Lock()
	ui.updateRel = rel
	ui.mu.Unlock()
	ui.window.Invalidate()
}

// doUpdate downloads and applies the pending release, then restarts.
func (ui *UI) doUpdate() {
	ui.mu.Lock()
	if ui.updateRel == nil || ui.updateBusy {
		ui.mu.Unlock()
		return
	}
	ui.updateBusy = true
	rel := ui.updateRel
	ui.mu.Unlock()

	ui.mu.Lock()
	ui.setBusyLocked("Скачивание обновления...")
	ui.mu.Unlock()
	ui.window.Invalidate()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := update.Apply(ctx, rel); err != nil {
		ui.mu.Lock()
		ui.updateBusy = false
		ui.setStatusLocked(fmt.Sprintf("Ошибка обновления: %v", err), false)
		ui.mu.Unlock()
		ui.window.Invalidate()
		return
	}

	ui.mu.Lock()
	ui.setStatusLocked("Обновлено! Перезапуск...", true)
	ui.mu.Unlock()
	ui.window.Invalidate()

	// Give the user a brief moment to see the message, then relaunch.
	time.Sleep(800 * time.Millisecond)
	if err := update.Restart(); err != nil {
		ui.mu.Lock()
		ui.updateBusy = false
		ui.setStatusLocked(fmt.Sprintf("Перезапустите вручную: %v", err), false)
		ui.mu.Unlock()
		ui.window.Invalidate()
	}
}

func (ui *UI) doInstall(force bool) {
	ui.mu.Lock()
	if ui.install == nil || ui.proxyInfo == nil || ui.busy {
		ui.mu.Unlock()
		return
	}
	ui.setBusyLocked("Установка...")
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
	msg := "Прокси установлен!"
	if reinstall {
		msg = "Прокси переустановлен!"
	}
	ui.finishStatus(msg, true, true)
}

func (ui *UI) doUninstall(force bool) {
	ui.mu.Lock()
	if ui.install == nil || ui.busy {
		ui.mu.Unlock()
		return
	}
	ui.setBusyLocked("Удаление...")
	install := ui.install
	ui.mu.Unlock()
	ui.window.Invalidate()

	d := deploy.New(ui.cfg, false, force)
	if err := d.Uninstall(install); err != nil {
		ui.finishStatus(fmt.Sprintf("Ошибка удаления: %v", err), false, false)
		return
	}
	ui.finishStatusInstalled(false, "Прокси удалён!", true)
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
	// Clear busy state when setting final status
	ui.busyMsg = ""
	ui.busyStart = time.Time{}
}

func (ui *UI) setBusyLocked(msg string) {
	ui.busy = true
	ui.busyMsg = msg
	ui.busyStart = time.Now()
	ui.statusMsg = ""
	ui.statusTime = time.Time{}
}

func (ui *UI) promptLayout(gtx layout.Context, phase uiPhase) layout.Dimensions {
	ui.mu.Lock()
	busy := ui.busy
	statusMsg := ui.statusMsg
	statusColor := ui.statusColor
	ui.mu.Unlock()

	if ui.btnPromptInstall.Clicked(gtx) && !busy {
		go ui.doSystemInstall(ui.chkDesktop.Value, ui.chkStartMenu.Value)
	}
	if ui.btnPromptNotNow.Clicked(gtx) && !busy {
		ui.mu.Lock()
		ui.phase = phaseMain
		ui.statusMsg = ""
		ui.mu.Unlock()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(30), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.H5(ui.theme, "Установка в систему")
				lbl.Color = colText
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(ui.divider),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(20), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(ui.theme, "Программу рекомендуется установить.")
				lbl.Color = colTextDim
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cb := material.CheckBox(ui.theme, &ui.chkDesktop, "Создать ярлык на Рабочем столе")
						cb.Color = colText
						cb.IconColor = colAccent
						cb.TextSize = unit.Sp(13)
						return cb.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						cb := material.CheckBox(ui.theme, &ui.chkStartMenu, "Создать ярлык в меню Пуск")
						cb.Color = colText
						cb.IconColor = colAccent
						cb.TextSize = unit.Sp(13)
						return cb.Layout(gtx)
					}),
				)
			})
		}),
		layout.Flexed(1, layout.Spacer{}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if statusMsg == "" {
				return layout.Dimensions{}
			}
			return ui.statusBannerDraw(gtx, statusMsg, statusColor)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(20), Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.theme, &ui.btnPromptInstall, "Установить")
							btn.CornerRadius = 8
							if busy {
								btn.Background = colDisabled
								btn.Color = colTextDim
								gtx = gtx.Disabled()
							} else {
								btn.Background = colAccent
								btn.Color = colText
							}
							return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, btn.Layout)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.theme, &ui.btnPromptNotNow, "Не сейчас")
							btn.CornerRadius = 8
							if busy {
								btn.Background = colDisabled
								btn.Color = colTextDim
								gtx = gtx.Disabled()
							} else {
								btn.Background = colAccent2
								btn.Color = colText
							}
							return btn.Layout(gtx)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.footer(gtx, phase)
		}),
	)
}

func (ui *UI) doSystemInstall(desktop, startMenu bool) {
	ui.mu.Lock()
	ui.setBusyLocked("Установка в систему...")
	ui.mu.Unlock()
	ui.window.Invalidate()

	if err := installer.InstallSelf(desktop, startMenu); err != nil {
		ui.mu.Lock()
		ui.busy = false
		ui.setStatusLocked(fmt.Sprintf("Ошибка установки: %v", err), false)
		ui.mu.Unlock()
		ui.window.Invalidate()
		return
	}

	ui.mu.Lock()
	ui.setStatusLocked("Установлено! Перезапуск...", true)
	ui.mu.Unlock()
	ui.window.Invalidate()

	time.Sleep(800 * time.Millisecond)
	if err := installer.RelaunchInstalled(); err != nil {
		ui.mu.Lock()
		ui.busy = false
		ui.phase = phaseMain
		ui.setStatusLocked(fmt.Sprintf("Ошибка запуска установленной копии: %v", err), false)
		ui.mu.Unlock()
		ui.window.Invalidate()
		return
	}
	// Installed copy launched; exit this portable instance.
	os.Exit(0)
}

// openURL opens a URL in the user's default browser on Windows via the native
// ShellExecute call. This avoids spawning a child process (and the associated
// conhost flicker / render-loop stall) entirely, and handles https/query
// strings reliably.
func openURL(u string) {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(u)
	if err != nil {
		return
	}
	_ = windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
