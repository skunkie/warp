package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jroimartin/gocui"

	"github.com/merzzzl/warp/internal/service"
	"github.com/merzzzl/warp/internal/utils/log"
)

const sidebarWidth = 22

type logWriter struct {
	logs chan string
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	select {
	case l.logs <- string(p):
	default:
	}

	return len(p), nil
}

func CreateTUI(routes *service.Routes, traffic *service.Traffic) error {
	lw := &logWriter{logs: make(chan string, 256)}

	log.Setup(lw, slog.LevelInfo)

	g, err := gocui.NewGui(gocui.Output256)
	if err != nil {
		return err
	}

	done := make(chan struct{})

	defer func() {
		close(done)
		g.Close()
	}()

	g.BgColor = gocui.ColorDefault
	g.FgColor = gocui.ColorDefault

	go animateBanner(g, done)

	g.SetManagerFunc(func(g *gocui.Gui) error {
		maxX, maxY := g.Size()

		if err := setupBanner(g, maxX); err != nil {
			return err
		}
		if err := setupUptime(g, done, maxX); err != nil {
			return err
		}
		if err := setupLogs(g, lw.logs, done, maxX, maxY); err != nil {
			return err
		}
		if err := setupPipes(g, done, maxX, maxY); err != nil {
			return err
		}
		if err := setupStats(g, traffic, done, maxX, maxY); err != nil {
			return err
		}
		if err := setupIPs(g, routes, done, maxX, maxY); err != nil {
			return err
		}
		return nil
	})

	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, func(*gocui.Gui, *gocui.View) error {
		return gocui.ErrQuit
	}); err != nil {
		return err
	}

	if err := g.MainLoop(); err != nil && !errors.Is(err, gocui.ErrQuit) {
		return err
	}

	return nil
}

func isNewView(err error) bool {
	return errors.Is(err, gocui.ErrUnknownView)
}

func colorize(s string, c int) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c, s)
}

func randomArt() string {
	arts := []string{
		"⊂(◉‿◉)つ",
		"( ✜︵ ✜ )",
		"ʕっ •ᴥ•ʔっ",
		"(｡◕‿‿◕｡)",
		"(っ ´ω`c)♡",
		"(ʘ‿ʘ)╯",
	}

	return arts[rand.Intn(len(arts))]
}

func animateBanner(g *gocui.Gui, done <-chan struct{}) {
	cl := 51

	ticker := time.NewTicker(75 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			cl++
			if cl > 231 {
				cl = 52
			}

			g.Update(func(*gocui.Gui) error {
				if v, err := g.View("banner"); err == nil {
					v.FgColor = gocui.Attribute(cl)
				}

				return nil
			})
		}
	}
}

func byteToSI(i float64) string {
	if i < 1024 {
		return fmt.Sprintf("%.2f B", i)
	}

	i /= 1024

	if i < 1024 {
		return fmt.Sprintf("%.2f KB", i)
	}

	i /= 1024

	if i < 1024 {
		return fmt.Sprintf("%.2f MB", i)
	}

	i /= 1024

	if i < 1024 {
		return fmt.Sprintf("%.2f GB", i)
	}

	i /= 1024

	return fmt.Sprintf("%.2f TB", i)
}
