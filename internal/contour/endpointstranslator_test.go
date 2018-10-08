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

package contour

import (
	"reflect"
	"sort"
	"testing"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/gogo/protobuf/proto"
	logrus "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

func TestEndpointsTranslatorAddEndpoints(t *testing.T) {
	tests := []struct {
		name string
		ep   *v1.Endpoints
		want []proto.Message
	}{{
		name: "simple",
		ep: eps("default", "simple", v1.EndpointSubset{
			Addresses: addresses("192.168.183.24"),
			Ports:     ports(8080),
		}),
		want: []proto.Message{
			clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080, 1)),
		},
	}, {
		name: "multiple addresses",
		ep: eps("default", "httpbin-org", v1.EndpointSubset{
			Addresses: addresses(
				"23.23.247.89",
				"50.17.192.147",
				"50.17.206.192",
				"50.19.99.160",
			),
			Ports: ports(80),
		}),
		want: []proto.Message{
			clusterloadassignment("default/httpbin-org",
				lbendpoint("23.23.247.89", 80, 1),
				lbendpoint("50.17.192.147", 80, 1),
				lbendpoint("50.17.206.192", 80, 1),
				lbendpoint("50.19.99.160", 80, 1),
			),
		},
	}}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			et := NewEndpointsTranslator(log, nodeWeightProvider(log))
			et.OnAdd(tc.ep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			endpoints := got[0].(*v2.ClusterLoadAssignment).GetEndpoints()[0].GetLbEndpoints()
			sort.Stable(endpointsByAddress(endpoints))

			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("got: %v, want: %v", got, tc.want)
			}
		})
	}
}

func TestEndpointsTranslatorRemoveEndpoints(t *testing.T) {
	tests := map[string]struct {
		setup func(*EndpointsTranslator)
		ep    *v1.Endpoints
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			ep: eps("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
		"remove different": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("default", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			ep: eps("default", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080, 1)),
			},
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: eps("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
		"remove long name": {
			setup: func(et *EndpointsTranslator) {
				e1 := eps(
					"super-long-namespace-name-oh-boy",
					"what-a-descriptive-service-name-you-must-be-so-proud",
					v1.EndpointSubset{
						Addresses: addresses(
							"172.16.0.1",
							"172.16.0.2",
						),
						Ports: ports(8000, 8443),
					},
				)
				et.OnAdd(e1)
			},
			ep: eps(
				"super-long-namespace-name-oh-boy",
				"what-a-descriptive-service-name-you-must-be-so-proud",
				v1.EndpointSubset{
					Addresses: addresses(
						"172.16.0.1",
						"172.16.0.2",
					),
					Ports: ports(8000, 8443),
				},
			),
			want: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(log, nodeWeightProvider(log))
			tc.setup(et)
			et.OnDelete(tc.ep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestEndpointsTranslatorRecomputeClusterLoadAssignment(t *testing.T) {
	tests := map[string]struct {
		oldep, newep *v1.Endpoints
		want         []proto.Message
	}{
		"simple": {
			newep: eps("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080, 1)),
			},
		},
		"multiple addresses": {
			newep: eps("default", "httpbin-org", v1.EndpointSubset{
				Addresses: addresses(
					"23.23.247.89",
					"50.17.192.147",
					"50.17.206.192",
					"50.19.99.160",
				),
				Ports: ports(80),
			}),
			want: []proto.Message{
				clusterloadassignment("default/httpbin-org",
					lbendpoint("23.23.247.89", 80, 1),
					lbendpoint("50.17.192.147", 80, 1),
					lbendpoint("50.17.206.192", 80, 1),
					lbendpoint("50.19.99.160", 80, 1),
				),
			},
		},
		"named container port": {
			newep: eps("default", "secure", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: []v1.EndpointPort{{
					Name: "https",
					Port: 8443,
				}},
			}),
			want: []proto.Message{
				clusterloadassignment("default/secure/https", lbendpoint("192.168.183.24", 8443, 1)),
			},
		},
		"remove existing": {
			oldep: eps("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(nil, nodeWeightProvider(nil))
			et.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))

			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}

// See #602
func TestEndpointsTranslatorScaleToZeroEndpoints(t *testing.T) {
	et := NewEndpointsTranslator(nil, nodeWeightProvider(nil))

	e1 := eps("default", "simple", v1.EndpointSubset{
		Addresses: addresses("192.168.183.24"),
		Ports:     ports(8080),
	})
	et.OnAdd(e1)

	// Assert endpoint was added
	want := []proto.Message{
		clusterloadassignment("default/simple", lbendpoint("192.168.183.24", 8080, 1)),
	}
	got := contents(et)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", want, got)
	}

	// e2 is the same as e1, but without endpoint subsets
	e2 := eps("default", "simple")
	et.OnUpdate(e1, e2)

	// Assert endpoints are removed
	want = []proto.Message{}
	got = contents(et)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("expected:\n%v\ngot:\n%v\n", want, got)
	}
}

func TestEndpointsTranslatorAddEndpointsNamespaceExludedFromClusterNames(t *testing.T) {
	tests := []struct {
		name string
		ep   []*v1.Endpoints
		want []proto.Message
	}{
		{
			name: "simple",
			ep: []*v1.Endpoints{
				eps("staging", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}),
				eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
					Ports:     ports(8080),
				})},
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 1),
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
	}

	log := testLogger(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			et := NewEndpointsTranslator(log, nodeWeightProvider(log))
			*et.ExcludeNamespaceFromServiceName = true
			for _, ep := range tc.ep {
				et.OnAdd(ep)
			}
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			endpoints := got[0].(*v2.ClusterLoadAssignment).GetEndpoints()[0].GetLbEndpoints()
			sort.Stable(endpointsByAddress(endpoints))

			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("got: %v, want: %v", got, tc.want)
			}
		})
	}
}

func TestEndpointsTranslatorRemoveEndpointsNamespaceExludedFromClusterNames(t *testing.T) {
	tests := map[string]struct {
		setup func(*EndpointsTranslator)
		ep    *v1.Endpoints
		want  []proto.Message
	}{
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
					Ports:     ports(8080),
				}))
			},
			ep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
		"remove different": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
					Ports:     ports(8080),
				}))
			},
			ep: eps("staging", "different", v1.EndpointSubset{
				Addresses: addresses("192.168.183.14"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 1),
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
		"remove non existent": {
			setup: func(*EndpointsTranslator) {},
			ep: eps("default", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{},
		},
	}

	log := testLogger(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(log, nodeWeightProvider(log))
			*et.ExcludeNamespaceFromServiceName = true
			tc.setup(et)
			et.OnDelete(tc.ep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("\nwant: %v\n got: %v", tc.want, got)
			}
		})
	}
}

func TestEndpointsTranslatorRecomputeClusterLoadAssignmentNamespaceExludedFromClusterNames(t *testing.T) {
	tests := map[string]struct {
		setup        func(*EndpointsTranslator)
		oldep, newep *v1.Endpoints
		want         []proto.Message
	}{
		"simple": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25"),
					Ports:     ports(8080),
				}))
			},
			newep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 1),
					lbendpoint("192.168.183.25", 8080, 1),
				),
			},
		},
		"multiple addresses": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
			},
			newep: eps("prod", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 1),
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
		"named container port": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
					Ports: []v1.EndpointPort{
						port(8080, "http"),
					},
				}))
			},
			newep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports: []v1.EndpointPort{
					port(8080, "http"),
				},
			}),
			want: []proto.Message{
				clusterloadassignment("simple/http",
					lbendpoint("192.168.183.24", 8080, 1),
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
		"remove existing": {
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.24"),
					Ports:     ports(8080),
				}))
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: addresses("192.168.183.25", "192.168.183.26", "192.168.183.27", "192.168.183.28", "192.168.183.29"),
					Ports:     ports(8080),
				}))
			},
			oldep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: addresses("192.168.183.24"),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.25", 8080, 1),
					lbendpoint("192.168.183.26", 8080, 1),
					lbendpoint("192.168.183.27", 8080, 1),
					lbendpoint("192.168.183.28", 8080, 1),
					lbendpoint("192.168.183.29", 8080, 1),
				),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			et := NewEndpointsTranslator(nil, nodeWeightProvider(nil))
			*et.ExcludeNamespaceFromServiceName = true
			tc.setup(et)

			if tc.oldep != nil && tc.newep != nil {
				et.OnUpdate(tc.oldep, tc.newep)
			} else if tc.oldep != nil {
				et.OnDelete(tc.oldep)
			} else if tc.newep != nil {
				et.OnAdd(tc.newep)
			}

			et.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			endpoints := got[0].(*v2.ClusterLoadAssignment).GetEndpoints()[0].GetLbEndpoints()
			sort.Stable(endpointsByAddress(endpoints))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}

func TestEndpointsTranslatorEndpointWeights(t *testing.T) {
	tests := map[string]struct {
		nodeWeights  map[string]int
		setup        func(*EndpointsTranslator)
		oldep, newep *v1.Endpoints
		want         []proto.Message
	}{
		"simple": {
			nodeWeights: map[string]int{
				"node1": 5,
				"node2": 10,
			},
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: epaddresses(address("192.168.183.25", "node1")),
					Ports:     ports(8080),
				}))
			},
			newep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: epaddresses(address("192.168.183.24", "node2")),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 10),
					lbendpoint("192.168.183.25", 8080, 5),
				),
			},
		},
		"multiple addresses": {
			nodeWeights: map[string]int{
				"node1": 5,
				"node2": 10,
				"node3": 20,
				"node4": 40,
				"node5": 80,
			},
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: epaddresses(address("192.168.183.24", "node2")),
					Ports:     ports(8080),
				}))
			},
			newep: eps("prod", "simple", v1.EndpointSubset{
				Addresses: epaddresses(
					address("192.168.183.25", "node1"),
					address("192.168.183.26", "node2"),
					address("192.168.183.27", "node3"),
					address("192.168.183.28", "node4"),
					address("192.168.183.29", "node5"),
				),
				Ports: ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.24", 8080, 10),
					lbendpoint("192.168.183.25", 8080, 5),
					lbendpoint("192.168.183.26", 8080, 10),
					lbendpoint("192.168.183.27", 8080, 20),
					lbendpoint("192.168.183.28", 8080, 40),
					lbendpoint("192.168.183.29", 8080, 80),
				),
			},
		},
		"named container port": {
			nodeWeights: map[string]int{
				"node1": 5,
				"node2": 10,
				"node3": 20,
				"node4": 40,
				"node5": 80,
			},
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: epaddresses(
						address("192.168.183.25", "node1"),
						address("192.168.183.26", "node2"),
						address("192.168.183.27", "node3"),
						address("192.168.183.28", "node4"),
						address("192.168.183.29", "node5"),
					),
					Ports: []v1.EndpointPort{
						port(8080, "http"),
					},
				}))
			},
			newep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: epaddresses(address("192.168.183.24", "node2")),
				Ports: []v1.EndpointPort{
					port(8080, "http"),
				},
			}),
			want: []proto.Message{
				clusterloadassignment("simple/http",
					lbendpoint("192.168.183.24", 8080, 10),
					lbendpoint("192.168.183.25", 8080, 5),
					lbendpoint("192.168.183.26", 8080, 10),
					lbendpoint("192.168.183.27", 8080, 20),
					lbendpoint("192.168.183.28", 8080, 40),
					lbendpoint("192.168.183.29", 8080, 80),
				),
			},
		},
		"remove existing": {
			nodeWeights: map[string]int{
				"node1": 5,
				"node2": 10,
				"node3": 20,
				"node4": 40,
				"node5": 80,
			},
			setup: func(et *EndpointsTranslator) {
				et.OnAdd(eps("staging", "simple", v1.EndpointSubset{
					Addresses: epaddresses(address("192.168.183.24", "node2")),
					Ports:     ports(8080),
				}))
				et.OnAdd(eps("prod", "simple", v1.EndpointSubset{
					Addresses: epaddresses(
						address("192.168.183.25", "node1"),
						address("192.168.183.26", "node2"),
						address("192.168.183.27", "node3"),
						address("192.168.183.28", "node4"),
						address("192.168.183.29", "node5"),
					),
					Ports: ports(8080),
				}))
			},
			oldep: eps("staging", "simple", v1.EndpointSubset{
				Addresses: epaddresses(address("192.168.183.24", "node2")),
				Ports:     ports(8080),
			}),
			want: []proto.Message{
				clusterloadassignment("simple",
					lbendpoint("192.168.183.25", 8080, 5),
					lbendpoint("192.168.183.26", 8080, 10),
					lbendpoint("192.168.183.27", 8080, 20),
					lbendpoint("192.168.183.28", 8080, 40),
					lbendpoint("192.168.183.29", 8080, 80),
				),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			nwp := nodeWeightProvider(nil).(*NodeWeightCache)
			nwp.nodeWeights = tc.nodeWeights
			et := NewEndpointsTranslator(nil, nwp)
			*et.ExcludeNamespaceFromServiceName = true
			tc.setup(et)

			if tc.oldep != nil && tc.newep != nil {
				et.OnUpdate(tc.oldep, tc.newep)
			} else if tc.oldep != nil {
				et.OnDelete(tc.oldep)
			} else if tc.newep != nil {
				et.OnAdd(tc.newep)
			}

			et.recomputeClusterLoadAssignment(tc.oldep, tc.newep)
			got := contents(et)
			sort.Stable(clusterLoadAssignmentsByName(got))
			endpoints := got[0].(*v2.ClusterLoadAssignment).GetEndpoints()[0].GetLbEndpoints()
			sort.Stable(endpointsByAddress(endpoints))
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("expected:\n%v\ngot:\n%v", tc.want, got)
			}
		})
	}
}

type clusterLoadAssignmentsByName []proto.Message

func (c clusterLoadAssignmentsByName) Len() int      { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool {
	return c[i].(*v2.ClusterLoadAssignment).ClusterName < c[j].(*v2.ClusterLoadAssignment).ClusterName
}

type endpointsByAddress []endpoint.LbEndpoint

func (ep endpointsByAddress) Len() int      { return len(ep) }
func (ep endpointsByAddress) Swap(i, j int) { ep[i], ep[j] = ep[j], ep[i] }
func (ep endpointsByAddress) Less(i, j int) bool {
	a1 := *ep[i].Endpoint.Address
	a2 := *ep[j].Endpoint.Address
	return a1.String() < a2.String()
}

func nodeWeightProvider(fieldLogger logrus.FieldLogger) NodeWeightProvider {
	nwp := NewNodeWeightProvider(fieldLogger).(*NodeWeightCache)
	nwp.DefaultNodeWeight = 1
	return nwp
}
