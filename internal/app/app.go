package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jovil96/balanza-agent/internal/api"
	"github.com/jovil96/balanza-agent/internal/config"
	"github.com/jovil96/balanza-agent/internal/desktop"
	"github.com/jovil96/balanza-agent/internal/printer"
	"github.com/jovil96/balanza-agent/internal/reader"
	"github.com/jovil96/balanza-agent/internal/scale"
	serialreader "github.com/jovil96/balanza-agent/internal/serial"
	"github.com/jovil96/balanza-agent/internal/storage"
	"github.com/jovil96/balanza-agent/internal/tcpscale"
)

// unifiedReader wraps any ScaleReader and exposes a Reconfigure method
// used by the desktop UI to restart the connection after settings change.
type unifiedReader struct {
	reader.ScaleReader
	reconfigure func(ctx context.Context, cfg *config.Config) error
}

func (u *unifiedReader) Reconfigure(ctx context.Context, cfg *config.Config) error {
	return u.reconfigure(ctx, cfg)
}

// Run starts the appropriate scale reader and local API.
// In headless mode it keeps those services alive without creating the Windows desktop interface.
func Run(headless bool) error {
	cfgPath := os.Getenv("BALANZA_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("cargar configuración: %w", err)
	}
	store, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("abrir almacenamiento: %w", err)
	}
	defer store.Close()

	ur, err := buildReader(cfg)
	if err != nil {
		return fmt.Errorf("configurar balanza: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ur.Start(ctx)
	scaleService := scale.New(ur, store)
	scaleService.Start(ctx)
	api.New(cfg.API, scaleService).Start(ctx)

	if headless {
		if !cfg.API.Enabled {
			return fmt.Errorf("el modo segundo plano requiere api.enabled: true")
		}
		<-ctx.Done()
		return nil
	}
	return desktop.Run(ctx, cfgPath, cfg, ur, scaleService, printer.New(cfg.Printer), store)
}

// buildReader constructs the right reader for the configured connection type.
func buildReader(cfg *config.Config) (*unifiedReader, error) {
	switch cfg.Connection.Type {
	case config.ConnectionTCP:
		r, err := tcpscale.NewReader(cfg)
		if err != nil {
			return nil, err
		}
		return &unifiedReader{
			ScaleReader: r,
			reconfigure: r.Reconfigure,
		}, nil

	case config.ConnectionUSB:
		// USB-CDC on Windows appears as a virtual COM port — reuse the serial reader
		// but pull connection parameters from the usb section.
		usbCfg := usbAsSerial(cfg)
		r, err := serialreader.NewReader(usbCfg)
		if err != nil {
			return nil, err
		}
		return &unifiedReader{
			ScaleReader: r,
			reconfigure: func(ctx context.Context, newCfg *config.Config) error {
				return r.Reconfigure(ctx, usbAsSerial(newCfg))
			},
		}, nil

	default: // config.ConnectionSerial
		r, err := serialreader.NewReader(cfg)
		if err != nil {
			return nil, err
		}
		return &unifiedReader{
			ScaleReader: r,
			reconfigure: r.Reconfigure,
		}, nil
	}
}

// usbAsSerial copies USB config fields into Serial so the serial reader can use them.
func usbAsSerial(cfg *config.Config) *config.Config {
	clone := *cfg
	clone.Serial = config.SerialConfig{
		Port:          cfg.USB.Port,
		BaudRate:      cfg.USB.BaudRate,
		DataBits:      cfg.USB.DataBits,
		Parity:        cfg.USB.Parity,
		StopBits:      cfg.USB.StopBits,
		ReadTimeoutMs: cfg.USB.ReadTimeoutMs,
	}
	return &clone
}
