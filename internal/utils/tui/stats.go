package tui

import (
	"fmt"
	"time"

	"github.com/jroimartin/gocui"

	"github.com/merzzzl/warp/internal/service"
)

func setupStats(g *gocui.Gui, traffic *service.Traffic, done <-chan struct{}, maxX, maxY int) error {
	v, err := g.SetView("stats", maxX-sidebarWidth+1, 3, maxX-1, 8)
	if err != nil {
		if !isNewView(err) {
			return err
		}

		v.Title = " Bandwidth "

		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					g.Update(func(*gocui.Gui) error {
						v.Clear()

						in, out := traffic.GetRates()
						sin, sout := traffic.GetTransferred()

						fmt.Fprintf(v, "In:  %s/s\n", byteToSI(in))
						fmt.Fprintf(v, "     %s\n", colorize(byteToSI(sin), 7))
						fmt.Fprintf(v, "Out: %s/s\n", byteToSI(out))
						fmt.Fprintf(v, "     %s\n", colorize(byteToSI(sout), 7))

						return nil
					})
				}
			}
		}()
	}

	return nil
}
