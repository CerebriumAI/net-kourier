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
	"testing"

	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	_ "knative.dev/pkg/system/testing"
)

func TestKourierConfig(t *testing.T) {
	configTests := []struct {
		name    string
		wantErr bool
		want    *Kourier
		data    map[string]string
	}{{
		name: "default configuration",
		want: DefaultConfig(),
		data: map[string]string{},
	}, {
		name: "disable logging",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: false,
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
		},
	}, {
		name:    "not a bool for logging",
		wantErr: true,
		data: map[string]string{
			enableServiceAccessLoggingKey: "foo",
		},
	}, {
		name: "enable proxy protocol, logging and internal cert",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
			EnableProxyProtocol:        true,
			ClusterCertSecret:          "my-cert",
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "true",
			enableProxyProtocol:           "true",
			clusterCert:                   "my-cert",
		},
	}, {
		name: "enable proxy protocol and disable logging, empty internal cert",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: false,
			EnableProxyProtocol:        true,
			ClusterCertSecret:          "",
			IdleTimeout:                0 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			enableProxyProtocol:           "true",
			clusterCert:                   "",
		},
	}, {
		name:    "not a bool for proxy protocol",
		wantErr: true,
		data: map[string]string{
			enableProxyProtocol: "foo",
		},
	}, {
		name: "set cipher suites",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: false,
			CipherSuites:               sets.New("foo", "bar"),
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			cipherSuites:                  "foo, bar",
		},
	}, {
		name: "set timeout to 200",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
			EnableProxyProtocol:        false,
			ClusterCertSecret:          "",
			IdleTimeout:                200 * time.Second,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "true",
			enableProxyProtocol:           "false",
			clusterCert:                   "",
			IdleTimeoutKey:                "200s",
		},
	}, {
		name: "add 3 trusted hops",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: false,
			TrustedHopsCount:           3,
		},
		data: map[string]string{
			enableServiceAccessLoggingKey: "false",
			trustedHopsCount:              "3",
		},
	}, {
		name: "configure tracing",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
			Tracing: Tracing{
				Enabled:           true,
				CollectorHost:     "jaeger.default.svc.cluster.local",
				CollectorPort:     9411,
				CollectorEndpoint: "/api/v2/spans",
			},
		},
		data: map[string]string{
			TracingCollectorFullEndpoint: "jaeger.default.svc.cluster.local:9411/api/v2/spans",
		},
	}, {
		name: "do not enable tracing",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
			Tracing: Tracing{
				Enabled: false,
			},
		},
		data: map[string]string{
			TracingCollectorFullEndpoint: "",
		},
	}, {
		name: "Enable use remote address",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
			UseRemoteAddress:           true,
		},
		data: map[string]string{
			useRemoteAddress: "true",
		},
	}, {
		name: "circuit breaker thresholds",
		want: &Kourier{
			MaxConnections:             20000,
			MaxPendingRequests:         30000,
			EnableServiceAccessLogging: true,
		},
		data: map[string]string{
			maxConnectionsKey:     "20000",
			maxPendingRequestsKey: "30000",
		},
	}, {
		// Envoy reads an explicit 0 as "allow nothing", never as "unlimited", so it is
		// treated as unset rather than passed straight through.
		name: "circuit breaker thresholds of zero fall back to defaults",
		want: &Kourier{
			MaxConnections:             DefaultMaxConnections,
			MaxPendingRequests:         DefaultMaxPendingRequests,
			EnableServiceAccessLogging: true,
		},
		data: map[string]string{
			maxConnectionsKey:     "0",
			maxPendingRequestsKey: "0",
		},
	}, {
		name:    "not a number for max connections",
		wantErr: true,
		data: map[string]string{
			maxConnectionsKey: "foo",
		},
	}}

	for _, tt := range configTests {
		t.Run(tt.name, func(t *testing.T) {
			actualCM, err := NewConfigFromConfigMap(&corev1.ConfigMap{
				Data: tt.data,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromConfigMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCM, tt.want); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}

			actualCfg, err := NewConfigFromMap(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfigFromMap() error = %v, WantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(actualCfg, actualCM); diff != "" {
				t.Errorf("Config mismatch: diff(-want,+got):\n%s", diff)
			}
		})
	}
}
