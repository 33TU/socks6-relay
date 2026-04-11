package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

func main() {
	proxyAddr := flag.String("proxy", "127.0.0.1:1080", "SOCKS5 proxy address") // host.docker.internal:1080
	target := flag.String("target", "http://ip6only.me/", "Target URL to fetch")
	timeout := flag.Int("timeout", 30, "Request timeout in seconds")
	logLevel := flag.Int("log-level", 0, "log level (-4=DEBUG, 0=INFO, 4=ERROR, 8=WARN)")
	flag.Parse()

	// Set log level
	slog.SetLogLoggerLevel(slog.Level(*logLevel))

	slog.Info("starting SOCKS5 proxy test", "proxy", *proxyAddr, "target", *target)

	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", *proxyAddr, nil, proxy.Direct)
	if err != nil {
		slog.Error("failed to create proxy dialer", "error", err)
		return
	}

	// Create HTTP transport with SOCKS5 proxy
	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	// Create HTTP client with custom transport and timeout
	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(*timeout) * time.Second,
	}

	slog.Info("making request through proxy", "proxy", *proxyAddr, "target", *target)

	// Make the request
	start := time.Now()
	resp, err := client.Get(*target)
	duration := time.Since(start)

	if err != nil {
		slog.Error("request failed", "error", err, "duration", duration)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read response", "error", err)
		return
	}

	slog.Info("request succeeded",
		"status", resp.Status,
		"duration", duration,
		"content_length", len(body),
	)

	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Response: %s\n", string(body))

	// Check if we got an IPv6 address in response
	if len(body) > 0 {
		slog.Info("proxy test completed successfully", "response_size", len(body))
	}
}
