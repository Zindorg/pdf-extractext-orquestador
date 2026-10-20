// Command server es el composition root del Orquestador: lee la configuración, arma los adaptadores
// y casos de uso, levanta el router de gin y apaga ordenadamente ante una señal.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/http_extractor"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/http_persister"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/http_summarizer"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/redis_streams"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/api"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/api/middleware"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/config"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/observability"
)

// shutdownGrace es cuánto se espera a que las peticiones en vuelo terminen antes de forzar la salida.
const shutdownGrace = 15 * time.Second

func main() {
	// La imagen distroless no trae shell ni cliente HTTP, así que el healthcheck del contenedor
	// reusa este mismo binario como sonda: `server healthcheck` consulta /health y sale según el
	// resultado.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error fatal:", err)
		os.Exit(1)
	}
}

// healthcheck consulta el liveness del propio proceso. No valida dependencias: para eso está /ready.
func healthcheck() error {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/health devolvió %d", resp.StatusCode)
	}
	return nil
}

// run arma todo y bloquea hasta que llega una señal. Devuelve un error solo si el arranque o el
// apagado fallan: un fallo de una petición no tumba el proceso.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := observability.New(cfg.LogLevel, os.Stdout)
	if err != nil {
		return err
	}

	extractor := http_extractor.New(cfg.ExtractorURL, cfg.ExtractorTimeout)
	summarizer := http_summarizer.New(cfg.SummarizerURL, cfg.SummarizerTimeout)
	// El proxy habla con la API del Persistidor, que responde rápido: no necesita el tiempo holgado
	// que sí exige el Resumidor, que serializa lento.
	persister := http_persister.New(cfg.PersisterURL, cfg.ExtractorTimeout)
	publisher := redis_streams.New(cfg.RedisAddr, cfg.StreamName)
	defer func() { _ = publisher.Close() }()

	router := api.NewRouter(api.Deps{
		Extract:       application.NewExtractUseCase(extractor, summarizer, publisher),
		Summary:       application.NewSummaryUseCase(persister, summarizer, publisher),
		Proxy:         application.NewProxyUseCase(persister),
		PublicBaseURL: cfg.PublicBaseURL,
		Ready:         publisher.Ready,
		Logger:        logger,
		RateLimit:     &middleware.RateLimitConfig{Limit: rateLimitPerWindow, Window: rateLimitWindow},
		ExtractGate:   middleware.NewSemaphore(cfg.ExtractMaxInFlight),
		SummaryGate:   middleware.NewSemaphore(cfg.SummaryMaxInFlight),
	})

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return serve(server, logger)
}

// serve levanta el servidor y lo apaga ordenadamente al recibir SIGINT o SIGTERM. El apagado deja
// terminar las peticiones en vuelo antes de salir.
func serve(server *http.Server, logger interface{ Info(string, ...any) }) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor escuchando", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("apagando ordenadamente")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("apagado completo")
	return nil
}

// rateLimitWindow y rateLimitPerWindow son el límite por IP del servicio sin autenticación
// (ADR-0007). El borde (Traefik) puede imponer el suyo; este es la última defensa por réplica.
const (
	rateLimitWindow    = time.Minute
	rateLimitPerWindow = 120
)
