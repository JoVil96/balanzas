package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/jovil96/balanza-agent/internal/app"
	"github.com/jovil96/balanza-agent/internal/winsvc"
)

// acquireInstanceMutex ensures only one GUI instance runs at a time.
// Returns false if another instance is already running.
func acquireInstanceMutex() bool {
	name, _ := windows.UTF16PtrFromString("Local\\BalanzaPOS_SingleInstance")
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil || windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return false
	}
	// Keep the handle alive for the lifetime of the process (intentional leak).
	_ = h
	return true
}

// bringExistingWindowToFront finds the existing GUI window and restores it.
func bringExistingWindowToFront() {
	user32 := windows.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	showWindow := user32.NewProc("ShowWindow")
	setForeground := user32.NewProc("SetForegroundWindow")

	title, _ := windows.UTF16PtrFromString("Balanza POS")
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd != 0 {
		showWindow.Call(hwnd, 9) // SW_RESTORE
		setForeground.Call(hwnd)
	}
}

func main() {
	// Service management commands (require elevated privileges)
	if len(os.Args) >= 2 {
		switch strings.ToLower(os.Args[1]) {
		case "install":
			exe, _ := os.Executable()
			if err := winsvc.Install(exe); err != nil {
				fmt.Fprintf(os.Stderr, "Error al instalar servicio: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Servicio instalado. Iniciando...")
			if err := winsvc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Advertencia al iniciar: %v\n", err)
			}
			fmt.Println("OK")
			return
		case "uninstall":
			_ = winsvc.Stop()
			if err := winsvc.Uninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "Error al desinstalar servicio: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Servicio desinstalado.")
			return
		case "start":
			if err := winsvc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Servicio iniciado.")
			return
		case "stop":
			if err := winsvc.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Servicio detenido.")
			return
		}
	}

	// Detect if running as a Windows Service
	isSvc, err := winsvc.IsWindowsService()
	if err != nil {
		log.Fatalf("no se pudo detectar modo de ejecución: %v", err)
	}

	if isSvc {
		configureBackgroundLog()
		log.Println("Balanza POS iniciada como servicio de Windows")
		if err := winsvc.RunAsService(func() error {
			return app.Run(true)
		}); err != nil {
			log.Fatalf("error en servicio: %v", err)
		}
		return
	}

	// Normal execution: GUI or headless
	headless := envEnabled("BALANZA_HEADLESS") || hasHeadlessFlag(os.Args[1:])
	if headless {
		configureBackgroundLog()
		log.Println("Balanza POS iniciada en segundo plano")
	} else {
		// Solo una instancia GUI a la vez; si ya hay una, traerla al frente y salir.
		if !acquireInstanceMutex() {
			bringExistingWindowToFront()
			return
		}
	}
	if err := app.Run(headless); err != nil {
		log.Fatal(err)
	}
}

func hasHeadlessFlag(args []string) bool {
	for _, arg := range args {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "--headless", "-headless", "/headless":
			return true
		}
	}
	return false
}

func envEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "si", "sí":
		return true
	default:
		return false
	}
}

func configureBackgroundLog() {
	file, err := os.OpenFile("balanza-agent.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}
