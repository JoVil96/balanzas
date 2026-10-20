package winsvc

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const ServiceName = "BalanzaPOS"
const ServiceDisplayName = "Balanza POS"
const ServiceDescription = "Agente de balanza: lee peso por puerto serie y expone API REST local."

// handler implements svc.Handler. It delegates execution to runFn.
type handler struct {
	runFn func() error
}

func (h handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	done := make(chan error, 1)
	go func() { done <- h.runFn() }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-done:
			if err != nil {
				log.Printf("balanza service error: %v", err)
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		}
	}
}

// RunAsService starts the Windows service control dispatcher.
// runFn is the headless application entry point.
func RunAsService(runFn func() error) error {
	return svc.Run(ServiceName, handler{runFn: runFn})
}

// IsWindowsService reports whether the process is running as a Windows service.
func IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

// Install registers the service with the Windows Service Control Manager.
// exePath must be the absolute path to the executable.
func Install(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("el servicio '%s' ya existe", ServiceName)
	}

	s, err = m.CreateService(ServiceName, exePath,
		mgr.Config{
			DisplayName:  ServiceDisplayName,
			Description:  ServiceDescription,
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
		},
		"--headless",
	)
	if err != nil {
		return fmt.Errorf("crear servicio: %w", err)
	}
	defer s.Close()

	_ = eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	return nil
}

// Uninstall removes the service from the Windows Service Control Manager.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("servicio no encontrado: %w", err)
	}
	defer s.Close()

	_ = eventlog.Remove(ServiceName)
	return s.Delete()
}

// Start starts the installed service.
func Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("servicio no encontrado: %w", err)
	}
	defer s.Close()

	return s.Start()
}

// Stop stops the running service.
func Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("servicio no encontrado: %w", err)
	}
	defer s.Close()

	st, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("detener servicio: %w", err)
	}

	timeout := time.Now().Add(10 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout esperando que el servicio se detenga")
		}
		time.Sleep(300 * time.Millisecond)
		st, err = s.Query()
		if err != nil {
			return fmt.Errorf("consultar estado: %w", err)
		}
	}
	return nil
}
