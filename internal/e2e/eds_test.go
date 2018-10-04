// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/gogo/protobuf/types"
	google_protobuf "github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// test that adding and removing endpoints don't leave turds
// in the eds cache.
func TestAddRemoveEndpoints(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// e1 is a simple endpoint for two hosts, and two ports
	// it has a long name to check that it's clustername is _not_
	// hashed.
	e1 := endpoints(
		"super-long-namespace-name-oh-boy",
		"what-a-descriptive-service-name-you-must-be-so-proud",
		v1.EndpointSubset{
			Addresses: addresses(
				"172.16.0.1",
				"172.16.0.2",
			),
			Ports: []v1.EndpointPort{{
				Name: "http",
				Port: 8000,
			}, {
				Name: "https",
				Port: 8443,
			}},
		},
	)

	rh.OnAdd(e1)

	// check that it's been translated correctly.
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, clusterloadassignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/http",
				lbendpoint("172.16.0.1", 8000, 1),
				lbendpoint("172.16.0.2", 8000, 1),
			)),
			any(t, clusterloadassignment(
				"super-long-namespace-name-oh-boy/what-a-descriptive-service-name-you-must-be-so-proud/https",
				lbendpoint("172.16.0.1", 8443, 1),
				lbendpoint("172.16.0.2", 8443, 1),
			)),
		},
		TypeUrl: endpointType,
		Nonce:   "0",
	}, streamEDS(t, cc))

	// remove e1 and check that the EDS cache is now empty.
	rh.OnDelete(e1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     endpointType,
		Nonce:       "0",
	}, streamEDS(t, cc))
}

// this example is generated by the combination of the service spec
// spec:
//   ports:
//   - name: foo
//     port: 80
//     protocol: TCP
//     targetPort: kuard
//   - name: admin
//     port: 9000
//     protocol: TCP
//     targetPort: 9000
//
// where the kuard target port is one of 8080 or 9999 depending on the
// matching pod spec.
func TestAddEndpointComplicated(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	e1 := endpoints(
		"default",
		"kuard",
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.77",
			),
			Ports: []v1.EndpointPort{{
				Name: "foo",
				Port: 9999,
			}},
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.78",
			),
			Ports: []v1.EndpointPort{{
				Name: "foo",
				Port: 8080,
			}},
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.77",
				"10.48.1.78",
			),
			Ports: []v1.EndpointPort{{
				Name: "admin",
				Port: 9000,
			}},
		},
	)

	rh.OnAdd(e1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, clusterloadassignment(
				"default/kuard/admin",
				lbendpoint("10.48.1.77", 9000, 1),
				lbendpoint("10.48.1.78", 9000, 1),
			)),
			any(t, clusterloadassignment(
				"default/kuard/foo",
				lbendpoint("10.48.1.77", 9999, 1), // TODO(dfc) order is not guaranteed by endpoint controller
				lbendpoint("10.48.1.78", 8080, 1),
			)),
		},
		TypeUrl: endpointType,
		Nonce:   "0",
	}, streamEDS(t, cc))
}

func TestEndpointFilter(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	// a single endpoint that represents several
	// cluster load assignments.
	e1 := endpoints(
		"default",
		"kuard",
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.77",
			),
			Ports: []v1.EndpointPort{{
				Name: "foo",
				Port: 9999,
			}},
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.78",
			),
			Ports: []v1.EndpointPort{{
				Name: "foo",
				Port: 8080,
			}},
		},
		v1.EndpointSubset{
			Addresses: addresses(
				"10.48.1.77",
				"10.48.1.78",
			),
			Ports: []v1.EndpointPort{{
				Name: "admin",
				Port: 9000,
			}},
		},
	)

	rh.OnAdd(e1)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, clusterloadassignment(
				"default/kuard/foo",
				lbendpoint("10.48.1.77", 9999, 1), // TODO(dfc) order is not guaranteed by endpoint controller
				lbendpoint("10.48.1.78", 8080, 1),
			)),
		},
		TypeUrl: endpointType,
		Nonce:   "0",
	}, streamEDS(t, cc, "default/kuard/foo"))

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		TypeUrl:     endpointType,
		Nonce:       "0",
	}, streamEDS(t, cc, "default/kuard/bar"))

}

// issue 602, test that an update from N endpoints
// to zero endpoints is handled correctly.
func TestIssue602(t *testing.T) {
	rh, cc, done := setup(t)
	defer done()

	e1 := endpoints("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports: []v1.EndpointPort{{
			Port: 8080,
		}},
	})
	rh.OnAdd(e1)

	// Assert endpoint was added
	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources: []types.Any{
			any(t, clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080, 1))),
		},
		TypeUrl: endpointType,
		Nonce:   "0",
	}, streamEDS(t, cc))

	// e2 is the same as e1, but without endpoint subsets
	e2 := endpoints("default", "simple")
	rh.OnUpdate(e1, e2)

	assertEqual(t, &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   []types.Any{},
		TypeUrl:     endpointType,
		Nonce:       "0",
	}, streamEDS(t, cc))
}

func streamEDS(t *testing.T, cc *grpc.ClientConn, rn ...string) *v2.DiscoveryResponse {
	t.Helper()
	rds := v2.NewEndpointDiscoveryServiceClient(cc)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	st, err := rds.StreamEndpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return stream(t, st, &v2.DiscoveryRequest{
		TypeUrl:       endpointType,
		ResourceNames: rn,
	})
}

func endpoints(ns, name string, subsets ...v1.EndpointSubset) *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Subsets: subsets,
	}
}

func addresses(ips ...string) []v1.EndpointAddress {
	var addrs []v1.EndpointAddress
	for _, ip := range ips {
		addrs = append(addrs, v1.EndpointAddress{IP: ip})
	}
	return addrs
}

func clusterloadassignment(name string, lbendpoints ...endpoint.LbEndpoint) *v2.ClusterLoadAssignment {
	return &v2.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints: []endpoint.LocalityLbEndpoints{{
			LbEndpoints: lbendpoints,
		}},
	}
}
func lbendpoint(addr string, port uint32, weight int) endpoint.LbEndpoint {
	lbep := endpoint.LbEndpoint{
		Endpoint: &endpoint.Endpoint{
			Address: &core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						Protocol: core.TCP,
						Address:  addr,
						PortSpecifier: &core.SocketAddress_PortValue{
							PortValue: port,
						},
					},
				},
			},
		},
	}

	if weight != 1 {
		lbep.LoadBalancingWeight = &google_protobuf.UInt32Value{
			Value: uint32(weight),
		}
	}

	return lbep
}
