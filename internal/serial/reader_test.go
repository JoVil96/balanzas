package serial

import (
	"github.com/jovil96/balanza-agent/internal/config"
	"testing"
)

func TestParseScaleReading(t *testing.T) {
	cfg := &config.Config{
		Parser: config.ParserConfig{Pattern: `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`, DefaultUnit: "kg"},
	}
	r, err := NewReader(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.parse("= 000.02")
	if !ok {
		t.Fatal("expected reading")
	}
	if got.Weight != 0.02 || got.Unit != "kg" || !got.Stable {
		t.Fatalf("unexpected reading: %+v", got)
	}
}

func TestNewReaderRejectsInvalidPattern(t *testing.T) {
	_, err := NewReader(&config.Config{Parser: config.ParserConfig{Pattern: "("}})
	if err == nil {
		t.Fatal("expected invalid regexp error")
	}
}

func TestParseLexusMatrixZeroReverseFrame(t *testing.T) {
	cfg := &config.Config{Parser: config.ParserConfig{
		Model: config.ScaleModelLexusMatrixZero, Pattern: `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`, DefaultUnit: "kg",
	}}
	r, err := NewReader(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.parse("=53.621")
	if !ok || got.Weight != 126.35 || got.Unit != "kg" || !got.Stable {
		t.Fatalf("unexpected Lexus Matrix reading: %+v, ok=%v", got, ok)
	}
}

func TestParseLexusMatrixZeroNormalFrame(t *testing.T) {
	cfg := &config.Config{Parser: config.ParserConfig{
		Model: config.ScaleModelLexusMatrixZero, Pattern: `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`, DefaultUnit: "kg",
	}}
	r, err := NewReader(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.parse("= 126.35")
	if !ok || got.Weight != 126.35 {
		t.Fatalf("unexpected Lexus Matrix normal reading: %+v, ok=%v", got, ok)
	}
}

func TestFrameBufferWaitsForCompleteReading(t *testing.T) {
	var b frameBuffer
	if frames := b.push([]byte("= 000.4"), false); len(frames) != 0 {
		t.Fatalf("partial reading must not be emitted: %v", frames)
	}
	frames := b.push([]byte("6\r\n"), false)
	if len(frames) != 1 || frames[0] != "= 000.46" {
		t.Fatalf("unexpected frames: %v", frames)
	}
}

func TestFrameBufferFlushesScaleWithoutLineEnding(t *testing.T) {
	var b frameBuffer
	b.push([]byte("= 000.46"), false)
	frames := b.push(nil, true)
	if len(frames) != 1 || frames[0] != "= 000.46" {
		t.Fatalf("unexpected frames: %v", frames)
	}
}

func TestParseCASCL5200NStableReading(t *testing.T) {
	cfg := &config.Config{
		Parser: config.ParserConfig{
			Model:       config.ScaleModelCASCL5200N,
			Pattern:     `(?P<stable>ST)\s+(?P<weight>\d+\.\d+)(?P<unit>kg|lb)`,
			DefaultUnit: "kg",
		},
	}
	r, err := NewReader(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		raw    string
		weight float64
		stable bool
	}{
		{"ST,GS,   0.005kg", 0.005, true},
		{"ST,GS,  15.320kg", 15.32, true},
		{"UN,GS,   5.100kg", 0, false},
	}
	for _, tc := range cases {
		got, ok := r.parse(tc.raw)
		if tc.stable {
			if !ok || got.Weight != tc.weight || got.Unit != "kg" || !got.Stable {
				t.Errorf("raw=%q: got %+v ok=%v", tc.raw, got, ok)
			}
		} else {
			if ok {
				t.Errorf("raw=%q: unstable frame must not parse as stable", tc.raw)
			}
		}
	}
}

func TestFrameBufferUsesNextReadingAsBoundary(t *testing.T) {
	var b frameBuffer
	frames := b.push([]byte("= 000.46= 000.47"), false)
	if len(frames) != 1 || frames[0] != "= 000.46" {
		t.Fatalf("unexpected frames: %v", frames)
	}
	frames = b.push([]byte("= 000.48"), false)
	if len(frames) != 1 || frames[0] != "= 000.47" {
		t.Fatalf("unexpected next frame: %v", frames)
	}
}
