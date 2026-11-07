// Command site-llama-swap serves the llama-swap.ai website.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mostlygeek/site-llama-swap.ai/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	listen := flag.String("listen", ":8080", "address to listen on")
	model := flag.String("model", server.DefaultModel, "Hugging Face repository of the Help agent's ONNX model")
	revision := flag.String("model-revision", server.DefaultModelRevision, "model repository commit or branch to load")
	transformers := flag.String("transformers-url", server.DefaultTransformersURL, "URL of the Transformers.js build the browser imports")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	srv, err := server.New(server.Options{
		Version:         version,
		Model:           *model,
		ModelRevision:   *revision,
		TransformersURL: *transformers,
	})
	if err != nil {
		slog.Error("starting server", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", *listen, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
}
