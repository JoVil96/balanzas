package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	ConnectionSerial = "serial"
	ConnectionTCP    = "tcp"
	ConnectionUSB    = "usb"
)

type ConnectionConfig struct {
	Type string `yaml:"type"` // serial | tcp | usb
}

type TCPConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	ReadTimeoutMs int    `yaml:"read_timeout_ms"`
}

type SerialConfig struct {
	Port          string `yaml:"port"`
	BaudRate      int    `yaml:"baud_rate"`
	DataBits      int    `yaml:"data_bits"`
	Parity        string `yaml:"parity"`
	StopBits      int    `yaml:"stop_bits"`
	ReadTimeoutMs int    `yaml:"read_timeout_ms"`
}

type USBConfig struct {
	// USB-CDC aparece en Windows como puerto COM virtual.
	// Usamos los mismos parámetros que serial pero en sección separada
	// para que la UI pueda mostrar campos distintos.
	Port          string `yaml:"port"`
	BaudRate      int    `yaml:"baud_rate"`
	DataBits      int    `yaml:"data_bits"`
	Parity        string `yaml:"parity"`
	StopBits      int    `yaml:"stop_bits"`
	ReadTimeoutMs int    `yaml:"read_timeout_ms"`
}

type ParserConfig struct {
	Model       string `yaml:"model,omitempty"`
	Pattern     string `yaml:"pattern"`
	DefaultUnit string `yaml:"default_unit"`
}

const (
	ScaleModelGeneric         = "generic"
	ScaleModelLexusMatrixZero = "lexus_matrix_zero"
	ScaleModelCASCL5200N      = "cas_cl5200n"
)


type PrinterConfig struct {
	Port     string `yaml:"port"`
	BaudRate int    `yaml:"baud_rate"`
	DataBits int    `yaml:"data_bits"`
	Parity   string `yaml:"parity"`
	StopBits int    `yaml:"stop_bits"`
	Company  string `yaml:"company"`
	RUC      string `yaml:"ruc"`
	Activity string `yaml:"activity"`
	Operator string `yaml:"operator"`
	Title    string `yaml:"title"`
	Footer   string `yaml:"footer"`
}

type StorageConfig struct {
	Path string `yaml:"path"`
}

type APIConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Address        string   `yaml:"address"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	APIKey         string   `yaml:"api_key,omitempty"`
}

type Config struct {
	Connection ConnectionConfig `yaml:"connection"`
	Serial     SerialConfig     `yaml:"serial"`
	USB        USBConfig        `yaml:"usb"`
	TCP        TCPConfig        `yaml:"tcp"`
	Parser     ParserConfig     `yaml:"parser"`
	Printer    PrinterConfig    `yaml:"printer"`
	Storage    StorageConfig    `yaml:"storage"`
	API        APIConfig        `yaml:"api"`
}

func defaults() *Config {
	return &Config{
		Connection: ConnectionConfig{Type: ConnectionSerial},
		Serial: SerialConfig{
			Port:          "COM3",
			BaudRate:      9600,
			DataBits:      8,
			Parity:        "N",
			StopBits:      1,
			ReadTimeoutMs: 500,
		},
		USB: USBConfig{
			Port:          "COM5",
			BaudRate:      9600,
			DataBits:      8,
			Parity:        "N",
			StopBits:      1,
			ReadTimeoutMs: 500,
		},
		TCP: TCPConfig{
			Host:          "192.168.1.100",
			Port:          3001,
			ReadTimeoutMs: 2000,
		},
		Parser: ParserConfig{
			Model:       ScaleModelGeneric,
			Pattern:     `(?P<stable>=)\s*(?P<weight>\d+\.\d+)`,
			DefaultUnit: "kg",
		},
		Printer: PrinterConfig{BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: 1, Company: "Mi empresa", Activity: "Exportadora de cacao", Title: "TICKET DE PESAJE", Footer: "Gracias por su trabajo"},
		Storage: StorageConfig{Path: "balanza.db"},
		API:     APIConfig{Enabled: true, Address: "127.0.0.1:8765", AllowedOrigins: []string{"http://127.0.0.1:5173/"}},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Connection.Type == "" {
		cfg.Connection.Type = ConnectionSerial
	}
	switch cfg.Connection.Type {
	case ConnectionSerial:
		if cfg.Serial.Port == "" {
			return nil, fmt.Errorf("serial.port es obligatorio")
		}
	case ConnectionUSB:
		if cfg.USB.Port == "" {
			return nil, fmt.Errorf("usb.port es obligatorio")
		}
	case ConnectionTCP:
		if cfg.TCP.Host == "" {
			return nil, fmt.Errorf("tcp.host es obligatorio")
		}
		if cfg.TCP.Port == 0 {
			return nil, fmt.Errorf("tcp.port es obligatorio")
		}
	default:
		return nil, fmt.Errorf("connection.type inválido: %q (use serial, usb o tcp)", cfg.Connection.Type)
	}
	if cfg.Storage.Path == "" {
		return nil, fmt.Errorf("storage.path es obligatorio")
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
