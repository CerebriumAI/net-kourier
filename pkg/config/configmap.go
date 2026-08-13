/*
Copyright 2021 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	cm "knative.dev/pkg/configmap"
)

const (
	// ConfigName is the name of config map for Kourier.
	ConfigName = "config-kourier"

	// enableServiceAccessLoggingKey is the config map key for enabling service related
	// access logging.
	enableServiceAccessLoggingKey = "enable-service-access-logging"

	// enableProxyProtocol is the config map key for enabling proxy protocol
	enableProxyProtocol = "enable-proxy-protocol"

	// clusterCert is the config map key for kourier internal certificates
	clusterCert = "cluster-cert-secret"

	// IdleTimeoutKey is the config map key for the amount of time that Kourier waits
	// for incoming requests. This value is set to "stream_idle_timeout" in Envoy.
	IdleTimeoutKey = "stream-idle-timeout"

	// enableCryptoMB is the config map for enabling CryptoMB private key provider.
	enableCryptoMB = "enable-cryptomb"

	// TracingCollectorFullEndpoint is the config map key to configure tracing at kourier gateway level
	TracingCollectorFullEndpoint = "tracing-collector-full-endpoint"

	disableEnvoyServerHeader = "disable-envoy-server-header"

	// maxConnectionsKey is the config map key for the per-cluster Envoy
	// "max_connections" circuit breaker threshold.
	maxConnectionsKey = "max-connections"

	// maxPendingRequestsKey is the config map key for the per-cluster Envoy
	// "max_pending_requests" circuit breaker threshold.
	maxPendingRequestsKey = "max-pending-requests"

	// maxRequestsKey is the config map key for the per-cluster Envoy
	// "max_requests" circuit breaker threshold.
	maxRequestsKey = "max-requests"
)

// Envoy defaults these to 1024, applied per cluster rather than per endpoint, and
// net-kourier leaves them unset. A single revision that goes not-ready under load parks
// its requests on held connections, exhausts the shared budget, and Envoy then sheds
// traffic for every revision behind that cluster with a 503 (UO). Adding replicas does
// not help — the threshold spans all endpoints in the cluster.
const (
	DefaultMaxConnections     = uint32(8192)
	DefaultMaxPendingRequests = uint32(8192)
	DefaultMaxRequests        = uint32(8192)
)

func DefaultConfig() *Kourier {
	return &Kourier{
		EnableServiceAccessLogging: true, // true is the default for backwards-compat
		EnableProxyProtocol:        false,
		ClusterCertSecret:          "",
		IdleTimeout:                0 * time.Second, // default value
		TrustedHopsCount:           0,
		CipherSuites:               nil,
		EnableCryptoMB:             false,
		UseRemoteAddress:           false,
		DisableEnvoyServerHeader:   false,
		MaxConnections:             DefaultMaxConnections,
		MaxPendingRequests:         DefaultMaxPendingRequests,
		MaxRequests:                DefaultMaxRequests,
	}
}

// NewConfigFromMap creates a DeploymentConfig from the supplied Map.
func NewConfigFromMap(configMap map[string]string) (*Kourier, error) {
	nc := DefaultConfig()

	if err := cm.Parse(configMap,
		cm.AsBool(enableServiceAccessLoggingKey, &nc.EnableServiceAccessLogging),
		cm.AsBool(enableProxyProtocol, &nc.EnableProxyProtocol),
		cm.AsString(clusterCert, &nc.ClusterCertSecret),
		cm.AsDuration(IdleTimeoutKey, &nc.IdleTimeout),
		cm.AsUint32(trustedHopsCount, &nc.TrustedHopsCount),
		cm.AsBool(useRemoteAddress, &nc.UseRemoteAddress),
		cm.AsStringSet(cipherSuites, &nc.CipherSuites),
		cm.AsBool(enableCryptoMB, &nc.EnableCryptoMB),
		asTracing(TracingCollectorFullEndpoint, &nc.Tracing),
		cm.AsBool(disableEnvoyServerHeader, &nc.DisableEnvoyServerHeader),
		cm.AsUint32(maxConnectionsKey, &nc.MaxConnections),
		cm.AsUint32(maxPendingRequestsKey, &nc.MaxPendingRequests),
		cm.AsUint32(maxRequestsKey, &nc.MaxRequests),
	); err != nil {
		return nil, err
	}

	// Zero disables the threshold entirely in Envoy, which is never what an operator
	// setting these means; treat it as "unset" rather than "unlimited".
	if nc.MaxConnections == 0 {
		nc.MaxConnections = DefaultMaxConnections
	}
	if nc.MaxPendingRequests == 0 {
		nc.MaxPendingRequests = DefaultMaxPendingRequests
	}
	if nc.MaxRequests == 0 {
		nc.MaxRequests = DefaultMaxRequests
	}

	return nc, nil
}

// Tracing contains all fields required to configure tracing at kourier gateway level.
// This object is mostly filled by the asTracing method, using TracingCollectorFullEndpoint value as the source.
type Tracing struct {
	Enabled           bool
	CollectorHost     string
	CollectorPort     uint16
	CollectorEndpoint string
}

func asTracing(collectorFullEndpoint string, tracing *Tracing) cm.ParseFunc {
	return func(data map[string]string) error {
		if raw, ok := data[collectorFullEndpoint]; ok && raw != "" {
			tracing.Enabled = true

			// We add a random scheme to be able to use url.ParseRequestURI.
			parsedURL, err := url.ParseRequestURI("scheme://" + raw)
			if err != nil {
				return fmt.Errorf("%q is not a valid URL: %w", raw, err)
			}

			tracing.CollectorHost = parsedURL.Hostname()
			collectorPortUint64, err := strconv.ParseUint(parsedURL.Port(), 10, 32)
			if err != nil {
				return fmt.Errorf("%q is not a valid port: %w", parsedURL.Port(), err)
			}

			if collectorPortUint64 > math.MaxUint16 {
				return fmt.Errorf("port %d must be a valid port", collectorPortUint64)
			}

			tracing.CollectorPort = uint16(collectorPortUint64)
			tracing.CollectorEndpoint = parsedURL.Path
		}

		return nil
	}
}

// NewConfigFromConfigMap creates a Kourier from the supplied configMap.
func NewConfigFromConfigMap(config *corev1.ConfigMap) (*Kourier, error) {
	return NewConfigFromMap(config.Data)
}

// Kourier includes the configuration for Kourier.
// +k8s:deepcopy-gen=true
type Kourier struct {
	// EnableServiceAccessLogging specifies whether requests reaching the Kourier gateway
	// should be logged.
	EnableServiceAccessLogging bool
	// EnableProxyProtocol specifies whether proxy protocol feature is enabled
	EnableProxyProtocol bool
	// ClusterCertSecret specifies the secret name for the server certificates of
	// Kourier Internal.
	ClusterCertSecret string
	// IdleTimeout specifies the amount of time that Kourier waits for incoming requests.
	// The default value is 5 minutes. This will not interfere any smaller configured
	// timeouts that may have existed in configurations prior to
	// this option, for example, the "timeoutSeconds" specified in Knative service is still
	// valid.
	IdleTimeout time.Duration
	// TrustedHopsCount configures the number of additional ingress proxy hops from the
	// right side of the x-forwarded-for HTTP header to trust.
	TrustedHopsCount uint32
	// UseRemoteAddress configures the connection manager to use the real remote address
	// of the client connection when determining internal versus external origin and manipulating various headers.
	UseRemoteAddress bool
	// EnableCryptoMB specifies whether Kourier enable CryptoMB private provider to accelerate
	// TLS handshake. The default value is "false".
	EnableCryptoMB bool
	// CipherSuites specifies the cipher suites for TLS external listener.
	CipherSuites sets.Set[string]
	// Tracing specifies the configuration for gateway tracing
	Tracing Tracing
	// Disable Server Header
	DisableEnvoyServerHeader bool
	// MaxConnections is the Envoy "max_connections" circuit breaker threshold applied to
	// every generated cluster, across all of that cluster's endpoints.
	MaxConnections uint32
	// MaxPendingRequests is the Envoy "max_pending_requests" circuit breaker threshold
	// applied to every generated cluster. Requests beyond it are shed with a 503.
	MaxPendingRequests uint32
	// MaxRequests is the Envoy "max_requests" circuit breaker threshold applied to every
	// generated cluster. The HTTP/1.1 connection pool checks it before creating a stream,
	// so it binds even though Envoy's docs frame it as an HTTP/2 control.
	MaxRequests uint32
}
