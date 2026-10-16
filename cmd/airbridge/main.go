package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"airbridge/internal/server"
)

func main() {
	cfg := server.Config{}
	flag.StringVar(&cfg.Address, "listen", ":8080", "HTTP listening address")
	flag.StringVar(&cfg.Directory, "data", "./data", "directory for received files")
	flag.StringVar(&cfg.Name, "name", "My Airbridge", "room name")
	flag.Int64Var(&cfg.MaxFile, "max-file", 50<<30, "maximum bytes per file")
	flag.Int64Var(&cfg.Quota, "quota", 100<<30, "maximum reserved bytes in the room")
	flag.Parse()
	if cfg.MaxFile <= 0 || cfg.Quota < cfg.MaxFile {
		log.Fatal("quota must be at least max-file; both must be positive")
	}
	code, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		log.Fatal(err)
	}
	cfg.PIN = fmt.Sprintf("%06d", code)
	// A stable PIN can be provided through the environment, never a command argument.
	if value := os.Getenv("AIRBRIDGE_PIN"); value != "" {
		cfg.PIN = value
	}
	if len(cfg.PIN) < 6 {
		log.Fatal("AIRBRIDGE_PIN must contain at least 6 characters")
	}
	app, err := server.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	srv := &http.Server{Addr: cfg.Address, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 2 * time.Minute, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
		}
	}()
	fmt.Printf("\nAirbridge • %s\n", cfg.Name)
	for _, url := range server.URLs(cfg.Address) {
		fmt.Println("Open", url)
	}
	fmt.Printf("Room code: %s\nFiles: %s\nKeep this computer awake while transferring.\n\n", cfg.PIN, cfg.Directory)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-stopped
}
