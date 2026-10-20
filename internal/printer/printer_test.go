package printer

import (
	"strings"
	"testing"
	"time"

	"github.com/jovil96/balanza-agent/internal/config"
)

func TestPreviewContainsTicketData(t *testing.T) {
	p := New(config.PrinterConfig{Company: "CACAO DEL ECUADOR", RUC: "0999999999001", Activity: "Exportadora de cacao", Title: "TICKET DE PESAJE", Footer: "Gracias"})
	preview := p.Preview(Ticket{
		Number: "000001234", Weight: 2.350, Unit: "kg", Operator: "María López",
		Product: "Cacao",
		Created: time.Date(2026, 9, 16, 8, 45, 32, 0, time.Local),
	})
	for _, want := range []string{"CACAO DEL ECUADOR", "0999999999001", "Exportadora de cacao", "16/09/2026", "08:45:32", "000001234", "María López", "Cacao", "2.350 kg", "5.181 lb"} {
		if !strings.Contains(preview, want) {
			t.Errorf("Preview() no contiene %q:\n%s", want, preview)
		}
	}
}
