// Package consul registra el servicio en el agente de Consul por su API HTTP. Es un cliente mínimo
// propio: el servicio solo necesita registrarse y darse de baja, y eso no justifica la dependencia
// del SDK oficial.
package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/adapters/httpclient"
)

// Registration describe la instancia que se anuncia en Consul.
type Registration struct {
	ID      string
	Name    string
	Address string
	Port    int
	Version string
	// CheckPath es la ruta HTTP que Consul consulta para decidir si la instancia está sana.
	CheckPath string
}

// Registrar habla con la API del agente de Consul.
type Registrar struct {
	httpClient *http.Client
	baseURL    string
	token      string
	reg        Registration
}

// New arma el registrador contra baseURL (por ejemplo http://consul:8500). token puede ir vacío
// cuando el ACL de Consul admite registros anónimos.
func New(baseURL, token string, reg Registration) *Registrar {
	return &Registrar{
		httpClient: httpclient.New(5 * time.Second),
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		reg:        reg,
	}
}

// ServiceID es el identificador de esta instancia en Consul.
func (r *Registrar) ServiceID() string { return r.reg.ID }

type serviceCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

type serviceDefinition struct {
	ID      string            `json:"ID"`
	Name    string            `json:"Name"`
	Address string            `json:"Address"`
	Port    int               `json:"Port"`
	Meta    map[string]string `json:"Meta,omitempty"`
	Check   serviceCheck      `json:"Check"`
}

// Register anuncia la instancia. El check lo ejecuta el agente de Consul desde su propio contenedor,
// así que la URL usa Address (el nombre del servicio en la red) y nunca localhost. Si el proceso muere
// sin darse de baja, Consul lo borra solo tras un minuto en estado crítico.
func (r *Registrar) Register(ctx context.Context) error {
	hostPort := r.reg.Address + ":" + strconv.Itoa(r.reg.Port)
	def := serviceDefinition{
		ID:      r.reg.ID,
		Name:    r.reg.Name,
		Address: r.reg.Address,
		Port:    r.reg.Port,
		Check: serviceCheck{
			HTTP:                           "http://" + hostPort + r.reg.CheckPath,
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}
	if r.reg.Version != "" {
		def.Meta = map[string]string{"version": r.reg.Version}
	}
	body, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("serializar el registro de Consul: %w", err)
	}
	return r.put(ctx, "/v1/agent/service/register", body)
}

// Deregister da de baja la instancia.
func (r *Registrar) Deregister(ctx context.Context) error {
	return r.put(ctx, "/v1/agent/service/deregister/"+url.PathEscape(r.reg.ID), nil)
}

func (r *Registrar) put(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, r.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("armar la petición a Consul: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("X-Consul-Token", r.token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("llamar a Consul: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("respuesta de Consul %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

// Maintain mantiene la instancia registrada hasta que ctx se cancele. Registra al arrancar (con
// espera creciente si Consul no responde) y después vuelve a registrar cada interval: el registro es
// idempotente, y así la instancia reaparece sola si Consul la dio de baja —por ejemplo, tras más de un
// minuto con /ready en 503 (DeregisterCriticalServiceAfter) o si Consul perdió su estado—.
// onRegistered se llama cada vez que se recupera el registro, no en cada renovación.
func (r *Registrar) Maintain(ctx context.Context, interval time.Duration, onRegistered func(), onError func(error)) {
	registered := false
	retry := time.Second
	for {
		wait := interval
		if err := r.Register(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			registered = false
			if onError != nil {
				onError(err)
			}
			wait = retry
			if retry < 30*time.Second {
				retry *= 2
			}
		} else {
			if !registered && onRegistered != nil {
				onRegistered()
			}
			registered = true
			retry = time.Second
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
