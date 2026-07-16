package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"suprasole-server/source"
)

func main() {
	port := flag.Int("port", 8000, "Port to listen on")
	flag.Parse()
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error binding listener: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Server listening on port %d\n", *port)
	if err := http.Serve(listener, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}
