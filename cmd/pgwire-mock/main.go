package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexandrmotologa/pgwire-mock/internal/admin"
	"github.com/alexandrmotologa/pgwire-mock/internal/config"
	"github.com/alexandrmotologa/pgwire-mock/internal/mock"
	"github.com/alexandrmotologa/pgwire-mock/internal/proxy"
	"github.com/alexandrmotologa/pgwire-mock/internal/server"
)

const Version = "1.0.0"

func main() {
	cfg, err := config.ParseFlags()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	if cfg.Version || cfg.ShowHelp {
		if cfg.Version {
			fmt.Printf("pgwire-mock v%s (PostgreSQL Frontend/Backend Protocol 3.0 Mock Server)\n", Version)
		}
		return
	}

	var rules []*mock.Rule

	// 1. Replay mode from .pgtape
	if cfg.ReplayPath != "" {
		tape, err := proxy.LoadTape(cfg.ReplayPath)
		if err != nil {
			log.Fatalf("failed to load tape: %v", err)
		}
		rules = proxy.ConvertTapeToRules(tape)
		log.Printf("[pgwire] loaded %d exchanges from tape %s", len(rules), cfg.ReplayPath)
	} else if cfg.RulesPath != "" {
		// 2. Rules loaded from YAML or JSON
		loaded, err := mock.LoadRulesFile(cfg.RulesPath)
		if err != nil {
			log.Fatalf("failed to load rules: %v", err)
		}
		rules = loaded
		log.Printf("[pgwire] loaded %d mock rules from %s", len(rules), cfg.RulesPath)
	}

	engine := mock.NewEngine(rules)

	pgAddr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	pgServer := server.NewServer(pgAddr, engine, cfg.Verbose)

	if err := pgServer.Start(); err != nil {
		log.Fatalf("failed to start pgwire server: %v", err)
	}

	// Start Admin API Server
	var adminServer *admin.Server
	if cfg.AdminPort > 0 {
		adminAddr := fmt.Sprintf("0.0.0.0:%d", cfg.AdminPort)
		adminServer = admin.NewServer(adminAddr, engine, pgServer, cfg.Verbose)
		if err := adminServer.Start(); err != nil {
			log.Fatalf("failed to start admin API server: %v", err)
		}
	}

	printStartupBanner(cfg.Port, cfg.AdminPort, len(rules))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[pgwire] shutting down server...")
	if adminServer != nil {
		_ = adminServer.Stop()
	}
	if err := pgServer.Stop(); err != nil {
		log.Printf("[pgwire] error during shutdown: %v", err)
	}
	log.Println("[pgwire] server stopped gracefully")
}

func printStartupBanner(pgPort, adminPort, ruleCount int) {
	fmt.Println("==============================================================")
	fmt.Printf(" PGWire-Mock v%s - PostgreSQL Protocol Mock Server\n", Version)
	fmt.Println("==============================================================")
	fmt.Printf(" PostgreSQL Mock Socket : localhost:%d\n", pgPort)
	if adminPort > 0 {
		fmt.Printf(" HTTP Admin & Assert API: http://localhost:%d\n", adminPort)
		fmt.Printf(" Prometheus Metrics     : http://localhost:%d/metrics\n", adminPort)
	}
	fmt.Printf(" Active Mock Rules      : %d\n", ruleCount)
	fmt.Println("--------------------------------------------------------------")
	fmt.Printf(" Connect with psql:\n")
	fmt.Printf("   psql -h localhost -p %d -U postgres -d testdb\n", pgPort)
	fmt.Println("==============================================================")
}
