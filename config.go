package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config groups the YAML-backed runtime settings.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Animations AnimationsConfig `yaml:"animations"`
	Messages   MessagesConfig   `yaml:"messages"`
}

// ServerConfig holds network and session-level controls.
type ServerConfig struct {
	Port                     int    `yaml:"port"`
	HostKeyPath              string `yaml:"host_key_path"`
	HandshakeTimeoutSeconds  int    `yaml:"handshake_timeout_seconds"`
	SessionTimeoutSeconds    int    `yaml:"session_timeout_seconds"`
	MaxConcurrentConnections int    `yaml:"max_concurrent_connections"`
}

// AnimationsConfig defines the timing and content used by the terminal presentation.
type AnimationsConfig struct {
	MatrixLines       []string `yaml:"matrix_lines"`
	MatrixSpeedMs     int      `yaml:"matrix_speed_ms"`
	ScanLinesCount    int      `yaml:"scan_lines_count"`
	ScanSpeedMs       int      `yaml:"scan_speed_ms"`
	AsciiGlitch       bool     `yaml:"ascii_glitch"`
	BootDelayMs       int      `yaml:"boot_delay_ms"`
	TypewriterSpeedMs int      `yaml:"typewriter_speed_ms"`
	ShowScanLines     bool     `yaml:"show_scan_lines"`
}

// MessagesConfig defines the user-facing strings rendered during the session.
type MessagesConfig struct {
	AccessGranted string `yaml:"access_granted"`
	PressToExit   string `yaml:"press_to_exit"`
	Goodbye       string `yaml:"goodbye"`
	HackMessage   string `yaml:"hack_message"`
}

var defaultConfig = Config{
	Server: ServerConfig{
		Port:                     2222,
		HostKeyPath:              "ssh_host_key",
		HandshakeTimeoutSeconds:  10,
		SessionTimeoutSeconds:    300,
		MaxConcurrentConnections: 100,
	},
	Animations: AnimationsConfig{
		MatrixLines: []string{
			"01001110 10101010 11110000 BINARY_INTRUSION_DETECTED",
			"10110101 01010101 00110011 SYSTEM_HACK_V.3.7.1",
			"01101001 11010101 10101010 NEURAL_LINK_ESTABLISHED",
			"11010010 01010101 11110000 ENCRYPTION_BYPASSED",
		},
		MatrixSpeedMs:     60,
		ScanLinesCount:    3,
		ScanSpeedMs:       5,
		AsciiGlitch:       true,
		BootDelayMs:       100,
		TypewriterSpeedMs: 10,
		ShowScanLines:     true,
	},
	Messages: MessagesConfig{
		AccessGranted: ">> ACCESS GRANTED <<",
		PressToExit:   "[Press 'q' to disconnect]",
		Goodbye:       "Connection terminated.",
		HackMessage:   "HACK THE PLANET!",
	},
}

// loadConfig reads config.yaml, applies defaults, and validates the final values.
func loadConfig(path string) (*Config, error) {
	cfg := defaultConfig

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyDefaults restores selected numeric defaults when the config explicitly uses zero values.
func (cfg *Config) applyDefaults() {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = defaultConfig.Server.Port
	}
	if cfg.Server.HostKeyPath == "" {
		cfg.Server.HostKeyPath = defaultConfig.Server.HostKeyPath
	}
	if cfg.Server.HandshakeTimeoutSeconds == 0 {
		cfg.Server.HandshakeTimeoutSeconds = defaultConfig.Server.HandshakeTimeoutSeconds
	}
	if cfg.Server.SessionTimeoutSeconds == 0 {
		cfg.Server.SessionTimeoutSeconds = defaultConfig.Server.SessionTimeoutSeconds
	}
	if cfg.Server.MaxConcurrentConnections == 0 {
		cfg.Server.MaxConcurrentConnections = defaultConfig.Server.MaxConcurrentConnections
	}
	if len(cfg.Animations.MatrixLines) == 0 {
		cfg.Animations.MatrixLines = defaultConfig.Animations.MatrixLines
	}
	if cfg.Animations.MatrixSpeedMs == 0 {
		cfg.Animations.MatrixSpeedMs = defaultConfig.Animations.MatrixSpeedMs
	}
	if cfg.Animations.ScanLinesCount == 0 {
		cfg.Animations.ScanLinesCount = defaultConfig.Animations.ScanLinesCount
	}
	if cfg.Animations.ScanSpeedMs == 0 {
		cfg.Animations.ScanSpeedMs = defaultConfig.Animations.ScanSpeedMs
	}
	if cfg.Animations.BootDelayMs == 0 {
		cfg.Animations.BootDelayMs = defaultConfig.Animations.BootDelayMs
	}
	if cfg.Animations.TypewriterSpeedMs == 0 {
		cfg.Animations.TypewriterSpeedMs = defaultConfig.Animations.TypewriterSpeedMs
	}
}

// validate rejects configuration values that would produce invalid network or animation settings.
func (cfg Config) validate() error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if cfg.Server.HostKeyPath == "" {
		return fmt.Errorf("server.host_key_path must not be empty")
	}
	if cfg.Server.HandshakeTimeoutSeconds < 0 {
		return fmt.Errorf("server.handshake_timeout_seconds must be zero or greater")
	}
	if cfg.Server.SessionTimeoutSeconds < 0 {
		return fmt.Errorf("server.session_timeout_seconds must be zero or greater")
	}
	if cfg.Server.MaxConcurrentConnections < 1 {
		return fmt.Errorf("server.max_concurrent_connections must be greater than zero")
	}
	if cfg.Animations.MatrixSpeedMs < 0 {
		return fmt.Errorf("animations.matrix_speed_ms must be zero or greater")
	}
	if cfg.Animations.ScanLinesCount < 0 {
		return fmt.Errorf("animations.scan_lines_count must be zero or greater")
	}
	if cfg.Animations.ScanSpeedMs < 0 {
		return fmt.Errorf("animations.scan_speed_ms must be zero or greater")
	}
	if cfg.Animations.BootDelayMs < 0 {
		return fmt.Errorf("animations.boot_delay_ms must be zero or greater")
	}
	if cfg.Animations.TypewriterSpeedMs < 0 {
		return fmt.Errorf("animations.typewriter_speed_ms must be zero or greater")
	}

	return nil
}
