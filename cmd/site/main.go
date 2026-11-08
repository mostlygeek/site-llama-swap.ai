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
	"runtime/debug"
	"syscall"
	"time"

	"github.com/mostlygeek/site-llama-swap.ai/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Log in JSON: the server's Logger nil falls through to slog.Default().
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	listen := flag.String("listen", ":8080", "address to listen on")
	model := flag.String("model", server.DefaultModel, "Hugging Face repository of the Help agent's GGUF model")
	revision := flag.String("model-revision", server.DefaultModelRevision, "model repository commit or branch to load")
	modelFile := flag.String("model-file", server.DefaultModelFile, "GGUF file in the model repository")
	tlsCert := flag.String("tls-cert-file", "", "TLS certificate file; serves HTTPS when set with -tls-key-file")
	tlsKey := flag.String("tls-key-file", "", "TLS private key file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if (*tlsCert == "") != (*tlsKey == "") {
		fmt.Fprintln(os.Stderr, "-tls-cert-file and -tls-key-file must be set together")
		os.Exit(2)
	}

	if *showVersion {
		fmt.Printf("%s (llama-swap %s)\n", version, llamaSwapVersion())
		return
	}

	srv, err := server.New(server.Options{
		Version:       version,
		Model:         *model,
		ModelRevision: *revision,
		ModelFile:     *modelFile,
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
		slog.Info("listening", "addr", *listen, "tls", *tlsCert != "", "version", version, "llama-swap", llamaSwapVersion())
		var err error
		if *tlsCert != "" {
			err = httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// llamaSwapVersion is the llama-swap module version the binary was built with.
// `make` builds against llama-swap's latest main, so this changes between
// builds.
func llamaSwapVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/mostlygeek/llama-swap" {
			return dep.Version
		}
	}
	return "unknown"
}
