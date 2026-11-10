// Command server es el composition root del Orquestador: lee la configuración, arma los adaptadores
// y casos de uso, levanta el router de gin y apaga ordenadamente ante una señal.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/consul"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/httpextractor"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/httppersister"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/httpsummarizer"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/redisstreams"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/api"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/api/middleware"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/application"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/config"
	"github.com/Zindorg/pdf-extractext-orquestador/internal/observability"
	webrouter "github.com/Zindorg/pdf-extractext-orquestador/internal/web/router"
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
	// Todo lo que use el logger por defecto (Recovery, librerías) sale en el mismo JSON.
	slog.SetDefault(logger)

	extractor := httpextractor.New(cfg.ExtractorURL, cfg.ExtractorTimeout)
	summarizer := httpsummarizer.New(cfg.SummarizerURL, cfg.SummarizerTimeout)
	persister := httppersister.New(cfg.PersisterURL, cfg.PersisterTimeout).
		WithMaxResponseBytes(cfg.PersisterMaxResponseBytes)
	publisher, err := redisstreams.New(redisstreams.Options{
		URL:    cfg.RedisURL,
		Stream: cfg.StreamName,
		MaxLen: cfg.StreamMaxLen,
	})
	if err != nil {
		return err
	}
	defer func() { _ = publisher.Close() }()

	extractUC := application.NewExtractUseCase(extractor, summarizer, publisher, persister).
		WithMaxDocumentBytes(cfg.MaxDocumentBytes)
	summaryUC := application.NewSummaryUseCase(persister, summarizer, publisher)
	proxyUC := application.NewProxyUseCase(persister)

	// gin arranca en modo debug, que imprime rutas y avisos en cada arranque. Es global al proceso.
	gin.SetMode(gin.ReleaseMode)

	router, err := api.NewRouter(api.Deps{
		Extract:       extractUC,
		Summary:       summaryUC,
		Proxy:         proxyUC,
		PublicBaseURL: cfg.PublicBaseURL,
		Ready:         publisher.Ready,
		Logger:        logger,
		RateLimit:     &middleware.RateLimitConfig{Limit: cfg.RateLimitPerMinute, Window: time.Minute},
		ExtractGate:   middleware.NewSemaphore(cfg.ExtractMaxInFlight),
		SummaryGate:   middleware.NewSemaphore(cfg.SummaryMaxInFlight),

		MaxDocumentBytes: cfg.MaxDocumentBytes,
		TrustedProxies:   cfg.TrustedProxies,
	})
	if err != nil {
		return err
	}

	if err := webrouter.Register(router, webrouter.Deps{
		Extract:        extractUC,
		Summary:        summaryUC,
		Proxy:          proxyUC,
		PublicBaseURL:  cfg.PublicBaseURL,
		Logger:         logger,
		MaxUploadBytes: cfg.MaxDocumentBytes,
	}); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		// Un cliente que sube el cuerpo muy lento no retiene la conexión (ni un cupo del semáforo).
		ReadTimeout: readTimeout,
		// La respuesta más larga es un resumen: el timeout del Resumidor más un margen.
		WriteTimeout: cfg.SummarizerTimeout + writeTimeoutMargin,
		IdleTimeout:  idleTimeout,
	}

	return serve(server, logger, newRegistrar(cfg, logger))
}

// newRegistrar arma el registro en Consul, o devuelve nil si CONSUL_HTTP_ADDR no está definida.
func newRegistrar(cfg config.Config, logger *slog.Logger) *consul.Registrar {
	if cfg.ConsulAddr == "" {
		logger.Info("registro en Consul desactivado: CONSUL_HTTP_ADDR vacía")
		return nil
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "local"
	}
	address := cfg.ServiceAddress
	if address == "" {
		address = hostname
	}
	return consul.New(cfg.ConsulAddr, cfg.ConsulToken, consul.Registration{
		// Nombre + hostname: cada réplica (cada contenedor) es una instancia distinta en Consul.
		ID:        cfg.ServiceName + "-" + hostname,
		Name:      cfg.ServiceName,
		Address:   address,
		Port:      cfg.HTTPPort,
		Version:   cfg.Version,
		CheckPath: "/ready",
	})
}

// serve levanta el servidor, lo anuncia en Consul y lo apaga ordenadamente al recibir SIGINT o
// SIGTERM. Al apagar, primero se da de baja en Consul —para que nadie nuevo lo descubra— y después
// deja terminar las peticiones en vuelo.
func serve(server *http.Server, logger *slog.Logger, registrar *consul.Registrar) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Se escucha antes de registrar: Consul no debe anunciar una instancia que todavía no atiende.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor escuchando", "addr", server.Addr)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	registerCtx, cancelRegister := context.WithCancel(ctx)
	defer cancelRegister()
	if registrar != nil {
		go func() {
			err := registrar.RegisterWithRetry(registerCtx, func(err error) {
				logger.Warn("no se pudo registrar en Consul, se reintenta", "error", err)
			})
			if err == nil {
				logger.Info("registrado en Consul", "service_id", registrar.ServiceID())
			}
		}()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("apagando ordenadamente")
	}

	cancelRegister()
	if registrar != nil {
		deregisterCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := registrar.Deregister(deregisterCtx); err != nil {
			logger.Warn("no se pudo dar de baja en Consul", "error", err)
		}
		cancel()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("apagado completo")
	return nil
}

// Timeouts del servidor HTTP. ReadTimeout cubre leer un documento de 10 MiB por una conexión lenta.
const (
	readTimeout        = 60 * time.Second
	writeTimeoutMargin = 30 * time.Second
	idleTimeout        = 120 * time.Second
)
