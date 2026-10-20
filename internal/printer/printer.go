package printer

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jovil96/balanza-agent/internal/config"
	seriallib "go.bug.st/serial"
)

// Ticket contains the same data used by both the preview and the physical print.
type Ticket struct {
	Number   string
	Weight   float64
	Unit     string
	Operator string
	Product  string
	Note     string
	Created  time.Time
}

// Printer sends ESC/POS tickets through a serial COM port.
type Printer struct{ cfg config.PrinterConfig }

func New(cfg config.PrinterConfig) *Printer { return &Printer{cfg: cfg} }

func (p *Printer) UpdateConfig(cfg config.PrinterConfig) { p.cfg = cfg }

// Preview is the source of truth for what is sent to the thermal printer.
func (p *Printer) Preview(ticket Ticket) string {
	if ticket.Created.IsZero() {
		ticket.Created = time.Now()
	}
	var text strings.Builder
	writeLine := func(value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintln(&text, value)
		}
	}
	writeLine(p.cfg.Company)
	if strings.TrimSpace(p.cfg.RUC) != "" {
		writeLine("RUC: " + p.cfg.RUC)
	}
	writeLine(p.cfg.Activity)
	writeLine(strings.Repeat("-", 36))
	writeLine(p.cfg.Title)
	fmt.Fprintf(&text, "Fecha       : %s\n", ticket.Created.Format("02/01/2006"))
	fmt.Fprintf(&text, "Hora        : %s\n", ticket.Created.Format("15:04:05"))
	writeField(&text, "Ticket N°", ticket.Number)
	writeField(&text, "Operador", ticket.Operator)
	writeField(&text, "Producto", ticket.Product)
	writeField(&text, "Nota", ticket.Note)
	fmt.Fprintln(&text, strings.Repeat("-", 36))
	fmt.Fprintln(&text, "             PESO NETO")
	fmt.Fprintf(&text, "             %.3f %s\n", ticket.Weight, ticket.Unit)
	if strings.EqualFold(strings.TrimSpace(ticket.Unit), "kg") {
		fmt.Fprintf(&text, "             (%.3f lb)\n", ticket.Weight*2.2046226218)
	}
	fmt.Fprintln(&text, strings.Repeat("-", 36))
	writeLine(p.cfg.Footer)
	writeLine(p.cfg.Company)
	return strings.TrimRight(text.String(), "\n")
}

func writeField(text *strings.Builder, label, value string) {
	if value = strings.TrimSpace(value); value != "" {
		fmt.Fprintf(text, "%-12s: %s\n", label, value)
	}
}

func (p *Printer) PrintTicket(data Ticket) error {
	if p.cfg.Port == "" {
		return fmt.Errorf("printer.port no está configurado")
	}
	mode := &seriallib.Mode{BaudRate: p.cfg.BaudRate, DataBits: p.cfg.DataBits, Parity: parity(p.cfg.Parity), StopBits: stopBits(p.cfg.StopBits)}
	port, err := seriallib.Open(p.cfg.Port, mode)
	if err != nil {
		return fmt.Errorf("abrir impresora %s: %w", p.cfg.Port, err)
	}
	defer port.Close()

	var output bytes.Buffer
	output.Write([]byte{0x1b, 0x40})
	output.WriteString(p.Preview(data))
	output.WriteString("\n\n\n")
	output.Write([]byte{0x1d, 0x56, 0x00})
	if _, err := port.Write(output.Bytes()); err != nil {
		return fmt.Errorf("escribir ticket: %w", err)
	}
	return nil
}

func parity(v string) seriallib.Parity {
	switch strings.ToUpper(v) {
	case "E":
		return seriallib.EvenParity
	case "O":
		return seriallib.OddParity
	default:
		return seriallib.NoParity
	}
}

func stopBits(v int) seriallib.StopBits {
	if v == 2 {
		return seriallib.TwoStopBits
	}
	return seriallib.OneStopBit
}
