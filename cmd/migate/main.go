package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/imzyb/MiGate/internal/web"
)

type panelConfig struct {
	PanelPort int    `json:"panel_port"`
	WebPath   string `json:"web_base_path"`
}

func main() {
	var host string
	var port int
	var configPath string
	flag.StringVar(&host, "host", "0.0.0.0", "bind host")
	flag.IntVar(&port, "port", 9999, "bind port")
	flag.StringVar(&configPath, "config", "", "panel config path")
	flag.Parse()

	if configPath != "" {
		cfg, err := readPanelConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config %s: %v\n", configPath, err)
			os.Exit(1)
		}
		if cfg.PanelPort > 0 {
			port = cfg.PanelPort
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("MiGate listening on %s", addr)
	if err := http.ListenAndServe(addr, web.NewRouter()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readPanelConfig(path string) (panelConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return panelConfig{}, err
	}
	var cfg panelConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return panelConfig{}, err
	}
	return cfg, nil
}
