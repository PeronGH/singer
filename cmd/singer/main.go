package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/PeronGH/singer/internal/gui"
	"github.com/PeronGH/singer/internal/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/icon.png
var appIconPNG []byte

func main() {
	var app *gui.App

	mygo.App.SetName("Singer")
	if runtime.GOOS != "android" {
		s, err := store.New()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		app, err = gui.New(s)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer app.Shutdown()

		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Role: mygo.RoleEditMenu},
			{Role: mygo.RoleWindowMenu},
		}))
		mygo.App.OnWindowAllClosed(func() {
			mygo.App.Quit()
		})
	}

	mygo.App.OnQuit(func() {
		if app != nil {
			app.Shutdown()
		}
	})
	if runtime.GOOS == "android" {
		mygo.App.OnDidResignActive(func() {
			if app != nil {
				app.SetBackgrounded(true)
			}
		})
		mygo.App.OnDidBecomeActive(func() {
			if app != nil {
				app.SetBackgrounded(false)
			}
		})
	}
	mygo.App.WhenReady(func() {
		if app == nil {
			baseDir, err := mygo.App.Path(mygo.PathAppData)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			s, err := store.NewAt(filepath.Join(baseDir, "singer"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			app, err = gui.New(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Singer",
			Width:     960,
			Height:    640,
			MinWidth:  360,
			MinHeight: 400,
			StateKey:  "main",
			Content:   ui.View(app.View),
		})
		if runtime.GOOS != "android" {
			_ = win.SetIcon(appIconPNG)
			_ = mygo.App.Dock.SetIcon(appIconPNG)
		}
		app.AttachWindow(win)
	})

	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
