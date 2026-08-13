/*
Copyright 2020 The Knative Authors

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

package envoy

import (
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	httpOptions "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	pkgconfig "knative.dev/net-kourier/pkg/config"
)

// ClusterOption customises a cluster built by NewCluster.
type ClusterOption func(*envoyclusterv3.Cluster)

// WithCircuitBreakers overrides the default per-cluster circuit breaker thresholds.
func WithCircuitBreakers(maxConnections, maxPendingRequests, maxRequests uint32) ClusterOption {
	return func(cluster *envoyclusterv3.Cluster) {
		cluster.CircuitBreakers = circuitBreakers(maxConnections, maxPendingRequests, maxRequests)
	}
}

// circuitBreakers builds thresholds for every routing priority. Envoy tracks each
// priority separately and routing priority is chosen per route, so omitting one leaves
// that priority on Envoy's 1024 default.
//
// max_requests must be raised alongside max_connections. Envoy's docs describe it as
// applicable to HTTP/2, but the HTTP/1.1 connection pool also checks it before creating
// a stream and sheds the request as an upstream_rq_pending_overflow when it is exceeded.
// Raising only max_connections just relocates the ceiling: measured on a dev gateway,
// raising connections alone moved 95k cx_overflow to 51k rq_pending_overflow at the
// same load, and 85% of requests were shed with a 503.
func circuitBreakers(maxConnections, maxPendingRequests, maxRequests uint32) *envoyclusterv3.CircuitBreakers {
	priorities := []envoycorev3.RoutingPriority{
		envoycorev3.RoutingPriority_DEFAULT,
		envoycorev3.RoutingPriority_HIGH,
	}

	thresholds := make([]*envoyclusterv3.CircuitBreakers_Thresholds, 0, len(priorities))
	for _, priority := range priorities {
		thresholds = append(thresholds, &envoyclusterv3.CircuitBreakers_Thresholds{
			Priority:           priority,
			MaxConnections:     wrapperspb.UInt32(maxConnections),
			MaxPendingRequests: wrapperspb.UInt32(maxPendingRequests),
			MaxRequests:        wrapperspb.UInt32(maxRequests),
		})
	}

	return &envoyclusterv3.CircuitBreakers{Thresholds: thresholds}
}

// NewCluster generates a new v3.Cluster with the given settings.
func NewCluster(
	name string,
	connectTimeout time.Duration,
	endpoints []*endpoint.LbEndpoint,
	isHTTP2 bool, transportSocket *envoycorev3.TransportSocket,
	discoveryType envoyclusterv3.Cluster_DiscoveryType,
	opts ...ClusterOption) *envoyclusterv3.Cluster {

	cluster := &envoyclusterv3.Cluster{
		Name: name,
		ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{
			Type: discoveryType,
		},
		ConnectTimeout: durationpb.New(connectTimeout),
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: endpoints,
			}},
		},
		TransportSocket: transportSocket,
		CircuitBreakers: circuitBreakers(
			pkgconfig.DefaultMaxConnections,
			pkgconfig.DefaultMaxPendingRequests,
			pkgconfig.DefaultMaxRequests,
		),
	}

	if isHTTP2 {
		protocolOpts, _ := anypb.New(&httpOptions.HttpProtocolOptions{
			UpstreamProtocolOptions: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_{
				ExplicitHttpConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig{
					ProtocolConfig: &httpOptions.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
				},
			},
		})

		cluster.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": protocolOpts,
		}
	}

	for _, opt := range opts {
		opt(cluster)
	}

	return cluster
}
