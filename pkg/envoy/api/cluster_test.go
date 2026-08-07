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
	"testing"
	"time"

	v3Cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/testing/protocmp"
	"gotest.tools/v3/assert"
)

func TestNewCluster(t *testing.T) {
	name := "myTestCluster_12345"
	connectTimeout := 5 * time.Second

	endpoint1 := NewLBEndpoint("127.0.0.1", 1234)
	endpoint2 := NewLBEndpoint("127.0.0.2", 1234)
	endpoints := []*endpoint.LbEndpoint{endpoint1, endpoint2}

	// With HTTP2
	c := NewCluster(name, connectTimeout, endpoints, true, nil, v3Cluster.Cluster_STATIC)
	assert.Equal(t, c.GetConnectTimeout().Seconds, int64(connectTimeout.Seconds()))
	assert.Assert(t, c.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] != nil)
	assert.Equal(t, c.GetName(), name)
	assert.DeepEqual(t, c.LoadAssignment.Endpoints[0].LbEndpoints, endpoints, protocmp.Transform())

	// Without HTTP2
	c = NewCluster(name, connectTimeout, endpoints, false, nil, v3Cluster.Cluster_STATIC)
	assert.Assert(t, c.TypedExtensionProtocolOptions["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"] == nil)
}

// Both priorities must carry thresholds: leaving either unset silently falls back to
// Envoy's 1024 default, which is the shared ceiling one noisy revision can exhaust for
// everyone. Values are asserted as literals, since comparing against the constants would
// still pass if they were reverted to 1024 — exactly the regression worth catching.
func TestNewClusterCircuitBreakers(t *testing.T) {
	tests := []struct {
		name               string
		opts               []ClusterOption
		maxConnections     uint32
		maxPendingRequests uint32
	}{{
		name:               "defaults when unconfigured",
		maxConnections:     5192,
		maxPendingRequests: 5192,
	}, {
		name:               "overridden from config",
		opts:               []ClusterOption{WithCircuitBreakers(77, 88)},
		maxConnections:     77,
		maxPendingRequests: 88,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := NewCluster("myTestCluster_12345", 5*time.Second,
				[]*endpoint.LbEndpoint{NewLBEndpoint("127.0.0.1", 1234)},
				false, nil, v3Cluster.Cluster_STATIC, test.opts...)

			thresholds := c.GetCircuitBreakers().GetThresholds()
			assert.Equal(t, len(thresholds), 2)

			byPriority := map[core.RoutingPriority]*v3Cluster.CircuitBreakers_Thresholds{}
			for _, th := range thresholds {
				byPriority[th.GetPriority()] = th
			}

			for _, priority := range []core.RoutingPriority{core.RoutingPriority_DEFAULT, core.RoutingPriority_HIGH} {
				th, ok := byPriority[priority]
				assert.Assert(t, ok, "no threshold for priority %v", priority)
				assert.Equal(t, th.GetMaxConnections().GetValue(), test.maxConnections)
				assert.Equal(t, th.GetMaxPendingRequests().GetValue(), test.maxPendingRequests)
			}
		})
	}
}
