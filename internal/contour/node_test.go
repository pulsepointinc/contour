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
	"testing"

	"github.com/google/go-cmp/cmp"
	logrus "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_cache "k8s.io/client-go/tools/cache"
)

func TestNodeWeightProvider(t *testing.T) {
	tests := map[string]struct {
		initialState         []*v1.Node
		nodeName             string
		nodeWeightAnnotation string
		defaultNodeWeight    int
		callHandler          bool
		old                  interface{}
		new                  interface{}
		want                 int
	}{
		"weight from annotation": {
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			want: 5,
		},
		"default weight if node name is missing": {
			callHandler:          false,
			nodeName:             "",
			nodeWeightAnnotation: "weight-annotation",
			defaultNodeWeight:    1,
			want:                 1,
		},
		"update weight from annotation": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "10",
						},
					},
				},
			},
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			old: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "10",
					},
				},
			},
			want: 5,
		},
		"delete weight from annotation": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "5",
						},
					},
				},
			},
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			old: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
		"abnormal weight from annotation": {
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "10000",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
		"unparsable weight from annotation": {
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "this will not parse as an int",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
		"annotation not found": {
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"not a weight-annotation": "5",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
		"wrong type added": {
			callHandler:          false,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "10000",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
		"wrong old type updated": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "10",
						},
					},
				},
			},
			callHandler:          false,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			old: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "10",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              10,
		},
		"wrong new type updated": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "5",
						},
					},
				},
			},
			callHandler:          false,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			new: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "10000",
					},
				},
			},
			old: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			want: 5,
		},
		"wrong type deleted": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "5",
						},
					},
				},
			},
			callHandler:          false,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			old: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"weight-annotation": "5",
					},
				},
			},
			defaultNodeWeight: 1,
			want:              5,
		},

		"delete final state unknown": {
			initialState: []*v1.Node{
				&v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "5",
						},
					},
				},
			},
			callHandler:          true,
			nodeName:             "node1",
			nodeWeightAnnotation: "weight-annotation",
			old: _cache.DeletedFinalStateUnknown{
				Key: "node1",
				Obj: &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Annotations: map[string]string{
							"weight-annotation": "5",
						},
					},
				},
			},
			defaultNodeWeight: 1,
			want:              1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			provider := NewNodeWeightProvider(logrus.New())

			cache := provider.(*NodeWeightCache)
			cache.NodeWeightAnnotation = tc.nodeWeightAnnotation
			cache.DefaultNodeWeight = tc.defaultNodeWeight

			if tc.initialState != nil {
				for _, node := range tc.initialState {
					cache.OnAdd(node)
				}
			}

			weightsChanged := false
			handler := func() {
				weightsChanged = true
			}
			provider.RegisterOnNodeWeightsChanged(handler)

			if tc.new != nil && tc.old != nil {
				cache.OnUpdate(tc.old, tc.new)
			} else if tc.new != nil {
				cache.OnAdd(tc.new)
			} else if tc.old != nil {
				cache.OnDelete(tc.old)
			}

			got := provider.GetNodeWeight(&tc.nodeName)

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Weight expected:\n%v\ngot:\n%v", tc.want, got)
			}

			if diff := cmp.Diff(tc.callHandler, weightsChanged); diff != "" {
				t.Fatalf("Handler called expected:\n%v\ngot:\n%v", tc.callHandler, weightsChanged)
			}
		})
	}
}
