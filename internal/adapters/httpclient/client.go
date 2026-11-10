// Package httpclient arma los clientes HTTP hacia los servicios internos con un pool de conexiones
// acorde a la concurrencia del Orquestador.
package httpclient

import (
	"net/http"
	"time"
)

// maxIdleConnsPerHost cubre el semáforo de extracción (16 por defecto) con margen. El default de Go
// es 2: con más peticiones en vuelo que eso, cada una abre y cierra su propia conexión.
const maxIdleConnsPerHost = 32

// New devuelve un cliente con el timeout dado y un transporte propio (no el global del proceso).
func New(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 4 * maxIdleConnsPerHost
	transport.MaxIdleConnsPerHost = maxIdleConnsPerHost
	transport.IdleConnTimeout = 90 * time.Second
	return &http.Client{Timeout: timeout, Transport: transport}
}
