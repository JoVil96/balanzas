// Package tcpscale implements a scale reader that connects to the CAS CL5200N
// (and compatible) over an Ethernet TCP socket.  The balanza exposes a raw TCP
// server (typically port 3001) that streams the same ASCII frames it would send
// over RS-232 — the frame parsing logic is therefore identical; only the
// transport layer changes.
package tcpscale

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jovil96/balanza-agent/internal/config"
	"github.com/jovil96/balanza-agent/internal/domain"
	"github.com/jovil96/balanza-agent/internal/reader"
)

// Reader connects to the scale over TCP and publishes parsed readings.
type Reader struct {
	cfg    *config.Config
	ch     chan domain.Reading
	re     *regexp.Regexp
	status chan reader.Status
	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewReader creates a TCP Reader.  Returns an error if the configured regex is invalid.
func NewReader(cfg *config.Config) (*Reader, error) {
	re, err := regexp.Compile(cfg.Parser.Pattern)
	if err != nil {
		return nil, fmt.Errorf("patrón inválido: %w", err)
	}
	return &Reader{
		cfg:    cfg,
		ch:     make(chan domain.Reading, 64),
		re:     re,
		status: make(chan reader.Status, 8),
	}, nil
}

func (r *Reader) Ch() <-chan domain.Reading      { return r.ch }
func (r *Reader) Status() <-chan reader.Status   { return r.status }

// Start launches the connection loop in a goroutine.
func (r *Reader) Start(ctx context.Context) {
	inner, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.cancel = cancel
	r.mu.Unlock()
	go r.loop(inner)
}

// Reconfigure applies new config and restarts the loop.
func (r *Reader) Reconfigure(ctx context.Context, cfg *config.Config) error {
	re, err := regexp.Compile(cfg.Parser.Pattern)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cfg = cfg
	r.re = re
	r.mu.Unlock()
	r.Start(ctx)
	return nil
}

func (r *Reader) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := r.readConn(ctx); err != nil {
			r.publishStatus(false, fmt.Sprintf("TCP sin conexión: %v", err))
			log.Printf("tcpscale error: %v — reconectando en 3s", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (r *Reader) readConn(ctx context.Context) error {
	r.mu.Lock()
	cfg := r.cfg
	r.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", cfg.TCP.Host, cfg.TCP.Port)
	timeout := time.Duration(cfg.TCP.ReadTimeoutMs) * time.Millisecond

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("conectar %s: %w", addr, err)
	}
	defer conn.Close()

	log.Printf("tcpscale: conectado a %s", addr)
	r.publishStatus(true, fmt.Sprintf("TCP %s conectado; esperando lectura...", addr))

	buf := make([]byte, 256)
	var fb frameBuffer

	for {
		if ctx.Err() != nil {
			return nil
		}
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			for _, frame := range fb.push(buf[:n], false) {
				r.publish(frame)
			}
		} else {
			for _, frame := range fb.push(nil, true) {
				r.publish(frame)
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout de lectura — flush y continuar
				for _, frame := range fb.push(nil, true) {
					r.publish(frame)
				}
				continue
			}
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
	case r.status <- reader.Status{Connected: connected, Message: message}:
	default:
	}
}

// --- frame buffering (identical logic to serial/reader.go) ---

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

// --- frame parsing (mirrors serial/reader.go) ---

func (r *Reader) parse(line string) (domain.Reading, bool) {
	r.mu.Lock()
	cfg := r.cfg
	re := r.re
	r.mu.Unlock()

	line = normalizeFrame(cfg.Parser.Model, line)
	match := re.FindStringSubmatch(line)
	if match == nil {
		return domain.Reading{}, false
	}
	names := re.SubexpNames()
	groups := make(map[string]string, len(names))
	for i, name := range names {
		if name != "" && i < len(match) {
			groups[name] = match[i]
		}
	}
	weightStr, ok := groups["weight"]
	if !ok || weightStr == "" {
		return domain.Reading{}, false
	}
	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		return domain.Reading{}, false
	}
	unit := groups["unit"]
	if unit == "" {
		unit = cfg.Parser.DefaultUnit
	}
	return domain.Reading{
		Weight:    weight,
		Unit:      unit,
		Stable:    groups["stable"] != "",
		Raw:       line,
		Timestamp: time.Now(),
	}, true
}

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
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 {
			return line
		}
		status := strings.TrimSpace(parts[0])
		if status != "ST" && status != "UN" {
			return line
		}
		rest := strings.TrimSpace(parts[2])
		return status + " " + rest
	}
	return line
}
