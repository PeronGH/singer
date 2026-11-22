package main

import (
	"fmt"
	"os"

	"github.com/PeronGH/singer/internal/gui"
	"github.com/PeronGH/singer/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	s, err := store.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	app, err := gui.New(s)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer app.Shutdown()

	mygo.App.SetName("Singer")
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	}))
	mygo.App.OnWindowAllClosed(func() {
		mygo.App.Quit()
	})
	mygo.App.OnQuit(app.Shutdown)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Singer",
			Width:     960,
			Height:    640,
			MinWidth:  640,
			MinHeight: 420,
			StateKey:  "main",
			Content:   ui.View(app.View),
		})
		app.AttachWindow(win)
	})

	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
