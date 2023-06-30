package broker

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/domain"
	"github.com/pivotal-cf/brokerapi/domain/apiresponses"
)

const (
	DefaultPort = "3000"
	DefaultHost = "0.0.0.0"

	DefaultLogLevel = "debug"
)

type Config struct {
	API      API
	Catalog  Catalog
	Provider []byte
}

func NewConfig(source io.Reader) (Config, error) {
	config := Config{}
	bytes, err := ioutil.ReadAll(source)
	if err != nil {
		return config, err
	}

	api := API{}
	if err = json.Unmarshal(bytes, &api); err != nil {
		return config, err
	}

	api.fillDefaults()
	err = api.validate()
	if err != nil {
		return config, err
	}

	api.LagerLogLevel, err = api.ConvertLogLevel()
	if err != nil {
		return config, err
	}

	catalog := Catalog{}
	if err = json.Unmarshal(bytes, &catalog); err != nil {
		return config, err
	}

	config = Config{
		API:      api,
		Catalog:  catalog,
		Provider: bytes,
	}

	err = config.Validate()

	return config, err
}

func (c Config) Validate() error {
	if reflect.DeepEqual(c.Catalog, Catalog{}) {
		return fmt.Errorf("Config error: catalog required")
	}
	if len(c.Catalog.Catalog.Services) == 0 {
		return fmt.Errorf("Config error: at least one service is required")
	}
	for _, service := range c.Catalog.Catalog.Services {
		if len(service.Plans) == 0 {
			return fmt.Errorf("Config error: no plans found for service %s", service.Name)
		}
	}
	return nil
}

type TLS struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
	CA          string `json:"ca"`
}

func (t *TLS) validate() error {
	if t.Certificate == "" {
		return fmt.Errorf("Config error: TLS certificate required")
	}
	if t.PrivateKey == "" {
		return fmt.Errorf("Config error: TLS private key required")
	}
	if t.CA == "" {
		return fmt.Errorf("Config error: TLS CA required")
	}
	return nil
}

func (t *TLS) Config() (*tls.Config, error) {
	certificate, err := tls.X509KeyPair([]byte(t.Certificate), []byte(t.PrivateKey))
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(&x509.Certificate{
		Raw: []byte(t.CA),
	})
	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      caPool,

		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},

		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}, nil
}

type API struct {
	BasicAuthUsername string `json:"basic_auth_username"`
	BasicAuthPassword string `json:"basic_auth_password"`
	Port              string `json:"port"`
	Host              string `json:"host"`
	TLS               *TLS   `json:"tls"`

	LogLevel      string `json:"log_level"`
	LagerLogLevel lager.LogLevel

	Locket *LocketConfig `json:"locket"`

	ContextTimeoutSeconds int `json:"context_timeout_seconds"`
}

func (api *API) ConvertLogLevel() (lager.LogLevel, error) {
	logLevels := map[string]lager.LogLevel{
		"DEBUG": lager.DEBUG,
		"INFO":  lager.INFO,
		"ERROR": lager.ERROR,
		"FATAL": lager.FATAL,
	}
	logLevel, ok := logLevels[strings.ToUpper(api.LogLevel)]
	if !ok {
		return lager.DEBUG, fmt.Errorf("Config error: log level %s does not map to a Lager log level", api.LogLevel)
	}
	return logLevel, nil
}

func (api *API) ContextTimeout() time.Duration {
	if api.ContextTimeoutSeconds == 0 {
		return DefaultContextTimeout
	}
	return time.Duration(api.ContextTimeoutSeconds) * time.Second
}

func (api *API) validate() error {
	if api.BasicAuthUsername == "" {
		return fmt.Errorf("Config error: basic auth username required")
	}
	if api.BasicAuthPassword == "" {
		return fmt.Errorf("Config error: basic auth password required")
	}

	if api.Locket != nil {
		if api.Locket.Address == "" {
			return fmt.Errorf("Config error: locket address required")
		}
	}

	if api.TLS != nil {
		return api.TLS.validate()
	}
	return nil
}

func (api *API) TLSEnabled() bool {
	return api.TLS != nil
}

func (api *API) fillDefaults() {
	if api.Port == "" {
		api.Port = DefaultPort
	}
	if api.Host == "" {
		api.Host = DefaultHost
	}
	if api.LogLevel == "" {
		api.LogLevel = DefaultLogLevel
	}
}

type Catalog struct {
	Catalog apiresponses.CatalogResponse `json:"catalog"`
}

func findServiceByID(catalog Catalog, serviceID string) (domain.Service, error) {
	for _, service := range catalog.Catalog.Services {
		if service.ID == serviceID {
			return service, nil
		}
	}
	return domain.Service{}, fmt.Errorf("Error: service %s not found in the catalog", serviceID)
}

func findPlanByID(service domain.Service, planID string) (domain.ServicePlan, error) {
	for _, plan := range service.Plans {
		if plan.ID == planID {
			return plan, nil
		}
	}
	return domain.ServicePlan{}, fmt.Errorf("Error: plan %s not found in service %s", planID, service.ID)
}
