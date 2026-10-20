package reader

import (
	"context"

	"github.com/jovil96/balanza-agent/internal/domain"
)

// Status describes the current connection state of a scale reader.
type Status struct {
	Connected bool
	Message   string
}

// ScaleReader is the common interface implemented by every transport
// (RS-232 serial, USB-CDC virtual COM, Ethernet TCP).
type ScaleReader interface {
	// Ch returns the channel on which parsed readings are published.
	Ch() <-chan domain.Reading
	// Status returns the channel on which connection status changes are published.
	Status() <-chan Status
	// Start begins reading in the background; it returns immediately.
	Start(ctx context.Context)
}
