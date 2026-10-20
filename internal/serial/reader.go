package serial

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"github.com/jovil96/balanza-agent/internal/config"
	"github.com/jovil96/balanza-agent/internal/domain"
	"github.com/jovil96/balanza-agent/internal/reader"
)

type Reading = domain.Reading

// Status is an alias for reader.Status for backward compatibility.
type Status = reader.Status

type Reader struct {
	cfg        *config.Config
	ch         chan Reading
	re         *regexp.Regexp
	status     chan Status
	mu         sync.Mutex
	cancelLoop context.CancelFunc
}

func NewReader(cfg *config.Config) (*Reader, error) {
	re, err := regexp.Compile(cfg.Parser.Pattern)
	if err != nil {
		return nil, err
	}
	return &Reader{
		cfg:    cfg,
		ch:     make(chan Reading, 64),
		re:     re,
		status: make(chan Status, 8),
	}, nil
}

func (r *Reader) Ch() <-chan Reading {
	return r.ch
}

func (r *Reader) Status() <-chan Status { return r.status }

func (r *Reader) Start(ctx context.Context) {
	inner, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancelLoop = cancel
	r.mu.Unlock()
	go r.loop(inner)
}

// Reconfigure applies new config and restarts the serial loop without
// restarting the whole app. The outer context from Start must still be alive.
func (r *Reader) Reconfigure(ctx context.Context, cfg *config.Config) error {
	re, err := regexp.Compile(cfg.Parser.Pattern)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cfg = cfg
	r.re = re
	cancel := r.cancelLoop
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.Start(ctx)
	return nil
}

func (r *Reader) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := r.readPort(ctx); err != nil {
			r.publishStatus(false, fmt.Sprintf("Sin conexión: %v", err))
			log.Printf("serial error: %v — reconnecting in 3s", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (r *Reader) readPort(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: r.cfg.Serial.BaudRate,
		DataBits: r.cfg.Serial.DataBits,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}
	switch strings.ToUpper(r.cfg.Serial.Parity) {
	case "E":
		mode.Parity = serial.EvenParity
	case "O":
		mode.Parity = serial.OddParity
	}
	if r.cfg.Serial.StopBits == 2 {
		mode.StopBits = serial.TwoStopBits
	}

	port, err := serial.Open(r.cfg.Serial.Port, mode)
	if err != nil {
		return err
	}
	defer port.Close()

	timeout := time.Duration(r.cfg.Serial.ReadTimeoutMs) * time.Millisecond
	port.SetReadTimeout(timeout)

	log.Printf("serial port %s opened", r.cfg.Serial.Port)
	r.publishStatus(true, fmt.Sprintf("Puerto %s conectado; esperando lectura...", r.cfg.Serial.Port))

	buf := make([]byte, 256)
	var frames frameBuffer

	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := port.Read(buf)
		if n > 0 {
			for _, frame := range frames.push(buf[:n], false) {
				r.publish(frame)
			}
		} else {
			// A serial timeout also marks the end of a frame for scales that do
			// not terminate their output with CR/LF.
			for _, frame := range frames.push(nil, true) {
				r.publish(frame)
			}
		}
		if err != nil {
			return err
		}
	}
}

func (r *Reader) publish(frame string) {
	if reading, ok := r.parse(frame); ok {
		select {
		case r.ch <- reading:
		default:
		}
	}
}

func (r *Reader) publishStatus(connected bool, message string) {
	select {
	case r.status <- Status{Connected: connected, Message: message}:
	default:
	}
}

type frameBuffer struct{ data []byte }

func (b *frameBuffer) push(chunk []byte, flush bool) []string {
	b.data = append(b.data, chunk...)
	var frames []string
	start := 0
	for i, value := range b.data {
		if value != '\r' && value != '\n' {
			continue
		}
		if frame := strings.TrimSpace(string(b.data[start:i])); frame != "" {
			frames = append(frames, frame)
		}
		start = i + 1
	}
	if start > 0 {
		b.data = append(b.data[:0], b.data[start:]...)
	}
	// Some scales stream continuously without CR/LF. The next '=' starts a
	// new measurement and therefore safely closes the preceding one.
	for len(b.data) > 1 {
		next := bytes.IndexByte(b.data[1:], '=')
		if next < 0 {
			break
		}
		split := next + 1
		if frame := strings.TrimSpace(string(b.data[:split])); frame != "" {
			frames = append(frames, frame)
		}
		b.data = append(b.data[:0], b.data[split:]...)
	}
	if flush {
		if frame := strings.TrimSpace(string(b.data)); frame != "" {
			frames = append(frames, frame)
		}
		b.data = b.data[:0]
	} else if len(b.data) > 1024 {
		b.data = b.data[:0]
	}
	return frames
}

func (r *Reader) parse(line string) (Reading, bool) {
	line = normalizeFrame(r.cfg.Parser.Model, line)
	match := r.re.FindStringSubmatch(line)
	if match == nil {
		return Reading{}, false
	}

	names := r.re.SubexpNames()
	groups := make(map[string]string, len(names))
	for i, name := range names {
		if name != "" && i < len(match) {
			groups[name] = match[i]
		}
	}

	weightStr, ok := groups["weight"]
	if !ok || weightStr == "" {
		return Reading{}, false
	}
	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		return Reading{}, false
	}

	unit := groups["unit"]
	if unit == "" {
		unit = r.cfg.Parser.DefaultUnit
	}

	return Reading{
		Weight:    weight,
		Unit:      unit,
		Stable:    groups["stable"] != "",
		Raw:       line,
		Timestamp: time.Now(),
	}, true
}

// normalizeFrame converts model-specific wire formats into the generic format
// consumed by the configured regular expression. A Lexus Matrix with Adr=99
// sends its six-character weight field in reverse order (for example
// "=53.621 " for 126.35). Adr=00 already sends the human-readable order.
//
// CAS CL5200N emits "ST,GS,   0.005kg" or "UN,GS,  15.320kg"; ST means
// stable, UN means unstable. The normalizer rewrites to "ST 0.005 kg" so the
// generic named-group pattern still applies.
func normalizeFrame(model, line string) string {
	switch model {
	case config.ScaleModelLexusMatrixZero:
		if !strings.HasPrefix(line, "=") {
			return line
		}
		payload := strings.TrimSuffix(strings.TrimPrefix(line, "="), " ")
		if len(payload) != 6 || strings.HasPrefix(payload, " ") {
			return line
		}
		reversed := []byte(payload)
		for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
			reversed[left], reversed[right] = reversed[right], reversed[left]
		}
		return "= " + string(reversed)

	case config.ScaleModelCASCL5200N:
		// Expected format: "ST,GS,   0.005kg" or "UN,GS,  15.320kg"
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 {
			return line
		}
		status := strings.TrimSpace(parts[0]) // "ST" or "UN"
		if status != "ST" && status != "UN" {
			return line
		}
		rest := strings.TrimSpace(parts[2]) // "0.005kg"
		return status + " " + rest
	}

	return line
}
