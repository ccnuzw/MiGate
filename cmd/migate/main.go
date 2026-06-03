package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/imzyb/MiGate/internal/web"
)

func main() {
	var host string
	var port int
	flag.StringVar(&host, "host", "0.0.0.0", "bind host")
	flag.IntVar(&port, "port", 9999, "bind port")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("MiGate listening on %s", addr)
	if err := http.ListenAndServe(addr, web.NewRouter()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
