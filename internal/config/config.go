package config

import (
	"flag"
	"fmt"
)

// Config holds runtime configuration options
type Config struct {
	Port       int
	AdminPort  int
	RulesPath  string
	Upstream   string
	RecordPath string
	ReplayPath string
	SSL        bool
	TLSCert    string
	TLSKey     string
	Stateful   bool
	Verbose    bool
	ShowHelp   bool
	Version    bool
}

// ParseFlags reads command-line flags into Config
func ParseFlags() (*Config, error) {
	cfg := &Config{}

	flag.IntVar(&cfg.Port, "port", 5432, "PostgreSQL mock TCP listen port")
	flag.IntVar(&cfg.Port, "p", 5432, "PostgreSQL mock TCP listen port (shorthand)")

	flag.IntVar(&cfg.AdminPort, "admin-port", 8080, "HTTP REST Admin, Dashboard and Assertion API port")

	flag.StringVar(&cfg.RulesPath, "rules", "", "Path to YAML or JSON mock rules file")
	flag.StringVar(&cfg.RulesPath, "r", "", "Path to YAML or JSON mock rules file (shorthand)")

	flag.StringVar(&cfg.Upstream, "upstream", "", "Upstream live PostgreSQL host:port for proxy mode")
	flag.StringVar(&cfg.RecordPath, "record", "", "Path to record network traffic into .pgtape file")
	flag.StringVar(&cfg.ReplayPath, "replay", "", "Path to replay offline mock session from .pgtape file")

	flag.BoolVar(&cfg.SSL, "ssl", false, "Enable TLS/SSL encryption for client connections")
	flag.StringVar(&cfg.TLSCert, "tls-cert", "", "Path to TLS certificate PEM file (auto-generated if empty)")
	flag.StringVar(&cfg.TLSKey, "tls-key", "", "Path to TLS private key PEM file")

	flag.BoolVar(&cfg.Stateful, "stateful", false, "Enable in-memory stateful table CRUD storage")

	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable detailed debug and query packet logs")
	flag.BoolVar(&cfg.Verbose, "v", false, "Enable detailed debug and query packet logs (shorthand)")

	flag.BoolVar(&cfg.Version, "version", false, "Print version information and exit")
	flag.BoolVar(&cfg.ShowHelp, "help", false, "Show help message")
	flag.BoolVar(&cfg.ShowHelp, "h", false, "Show help message (shorthand)")

	flag.Usage = func() {
		fmt.Printf("Usage: pgwire-mock [options]\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.ShowHelp {
		flag.Usage()
		return cfg, nil
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", cfg.Port)
	}

	return cfg, nil
}
