package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/jroimartin/gocui"

	"github.com/merzzzl/warp/internal/service"
)

func setupIPs(g *gocui.Gui, routes *service.Routes, done <-chan struct{}, maxX, maxY int) error {
	v, err := g.SetView("ips", maxX-sidebarWidth+1, 9, maxX-1, maxY-1)
	if err != nil {
		if !isNewView(err) {
			return err
		}

		v.Title = " IP List "

		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					g.Update(func(*gocui.Gui) error {
						v.Clear()

						ips := routes.GetAll()
						sort.Strings(ips)

						for _, ip := range ips {
							fmt.Fprintln(v, ip)
						}

						return nil
					})
				}
			}
		}()
	}

	return nil
}
