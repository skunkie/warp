package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jroimartin/gocui"

	"github.com/merzzzl/warp/internal/utils/network"
)

func setupPipes(g *gocui.Gui, done <-chan struct{}, maxX, maxY int) error {
	v, err := g.SetView("pipes", 0, maxY-15, maxX-sidebarWidth, maxY-1)
	if err != nil {
		if !isNewView(err) {
			return err
		}

		v.Title = " Pipes "

		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					g.Update(func(*gocui.Gui) error {
						v.Clear()

						list := network.List()

						sort.Slice(list, func(i, j int) bool {
							return list[i].OpenCount > list[j].OpenCount
						})

						for _, pipe := range list {
							fmt.Fprintf(v, "%s %s %s %s %s %s\n",
								colorize(time.Unix(0, 0).UTC().Add(time.Since(pipe.OpenAt)).Format("15:04:05"), 7),
								fmt.Sprintf("%.3d", pipe.OpenCount),
								colorize(strings.Repeat("»", 3), 6),
								colorize(strings.ToUpper(pipe.Dest.Network()), 11),
								pipe.Dest.String(),
								colorize(pipe.Protocol, 6),
							)
						}

						return nil
					})
				}
			}
		}()
	}

	return nil
}
