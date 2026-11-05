package main

import (
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"bibliocell"
	"bibliocell/internal/settings"
)

// keepInBackground makes closing the main window hide it instead of quitting,
// unless the person turned on QuitOnWindowClose. The app keeps running, so
// the MCP server and source updates stay available; the tray icon, the Dock
// icon (macOS) or launching the app again brings the window back, and the
// tray menu or Cmd+Q quits for real.
func keepInBackground(win *mygo.Window, store *settings.Store) {
	// A quit (menu, tray, relaunch, update) starts before the windows close,
	// so OnClose can tell those closes from the person's own. Both listeners
	// run on the main thread.
	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })

	win.OnClose(func(e *mygo.CloseEvent) {
		if quitting || store.Get().QuitOnWindowClose {
			return
		}
		e.PreventDefault()
		win.Hide()
	})

	show := func() {
		if win.IsMinimized() {
			win.Restore()
		}
		win.Show()
		win.Focus()
		mygo.App.Focus()
	}
	// macOS: a click on the Dock icon while the window is hidden.
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			show()
		}
	})
	// Launching the app again shows the running one rather than starting a
	// second instance, which could not take the MCP port.
	mygo.App.OnSecondInstance(func([]string, string) { show() })

	// The Dock already reaches a hidden window on macOS; elsewhere a hidden
	// window leaves the taskbar, so the tray is the way back.
	if runtime.GOOS != "darwin" {
		addTray(show)
	}
}

// addTray puts BiblioCell in the notification area: a click shows the window,
// and the menu (a right-click on Windows) shows it or quits.
func addTray(show func()) {
	open, quit := trayLabels()
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: open, Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
		mygo.Separator(),
		{Label: quit, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	options := mygo.TrayOptions{ID: "bibliocell", Icon: bibliocell.TrayIcon, ToolTip: "BiblioCell"}
	// AppIndicator (Linux) always shows a menu; on Windows a menu would take
	// the left click, so it opens only on a right-click.
	if runtime.GOOS == "linux" {
		options.Menu = menu
	}
	tray, err := mygo.NewTray(options)
	if err != nil {
		log.Printf("tray icon unavailable: %v", err)
		return
	}
	tray.OnClick(show)
	if runtime.GOOS != "linux" {
		tray.OnRightClick(func() {
			tray.SetMenu(menu)
			tray.PopUpMenu()
			tray.SetMenu(nil)
		})
	}
}

// trayLabels follows the system language, as the page's own default does.
func trayLabels() (open, quit string) {
	languages := mygo.App.PreferredLanguages()
	if len(languages) == 0 || strings.HasPrefix(strings.ToLower(languages[0]), "zh") {
		return "打开 BiblioCell", "退出 BiblioCell"
	}
	return "Open BiblioCell", "Quit BiblioCell"
}
