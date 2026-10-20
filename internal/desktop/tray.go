package desktop

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// setupTray creates the system tray icon and wires window-close behavior.
// When minimizeToTray is true, closing the window hides it instead of exiting.
func setupTray(mw *walk.MainWindow, minimizeToTray *bool) (*walk.NotifyIcon, error) {
	icon, err := walk.NewIconFromSysDLL("shell32", 133)
	if err != nil {
		icon, err = walk.NewIconFromSysDLL("shell32", 0)
		if err != nil {
			return nil, err
		}
	}

	ni, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return nil, err
	}

	if err := ni.SetIcon(icon); err != nil {
		ni.Dispose()
		return nil, err
	}
	if err := ni.SetToolTip("Balanza POS — corriendo en segundo plano"); err != nil {
		ni.Dispose()
		return nil, err
	}
	if err := ni.SetVisible(true); err != nil {
		ni.Dispose()
		return nil, err
	}

	// Left-click on tray icon → show window
	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			showWindow(mw)
		}
	})

	// Context menu
	menu := ni.ContextMenu()

	abrir := walk.NewAction()
	abrir.SetText("Abrir Balanza POS")
	abrir.Triggered().Attach(func() { showWindow(mw) })

	salir := walk.NewAction()
	salir.SetText("Salir completamente")
	salir.Triggered().Attach(func() {
		*minimizeToTray = false
		mw.Close()
	})

	menu.Actions().Add(abrir)
	menu.Actions().Add(walk.NewSeparatorAction())
	menu.Actions().Add(salir)

	// Override window close: hide to tray instead of exiting
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if *minimizeToTray {
			*canceled = true
			mw.Hide()
			ni.ShowInfo("Balanza POS", "La aplicación sigue activa en la bandeja del sistema.")
		}
	})

	return ni, nil
}

func showWindow(mw *walk.MainWindow) {
	hwnd := mw.Handle()
	// SW_RESTORE = 9: restores minimized/maximized window to normal
	win.ShowWindow(hwnd, win.SW_RESTORE)
	win.SetForegroundWindow(hwnd)
}
