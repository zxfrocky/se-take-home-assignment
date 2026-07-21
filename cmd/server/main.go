package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/feedme/se-take-home-assignment/internal/order"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "HTTP listen address")
	resultPath := flag.String("result", "scripts/result.txt", "result file path")
	processing := flag.Duration("processing-duration", durationFromEnv("ORDER_PROCESSING_DURATION", 10*time.Second), "real processing duration per order")
	flag.Parse()

	file, err := os.Create(*resultPath)
	if err != nil {
		log.Fatalf("create result file: %v", err)
	}
	defer file.Close()

	logger := order.NewEventLogger(file)
	controller := order.NewController(logger, *processing)
	defer controller.Shutdown()

	server := &http.Server{Addr: *addr, Handler: order.NewHTTPServer(controller)}
	log.Printf("order server listening on http://%s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	if ms, err := strconv.Atoi(value); err == nil {
		return time.Duration(ms) * time.Millisecond
	}
	return fallback
}
