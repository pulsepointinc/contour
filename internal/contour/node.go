package contour

import (
	"strconv"

	logrus "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_cache "k8s.io/client-go/tools/cache"
)

type NodeWeightProvider struct {
	logrus.FieldLogger
	NodeWeightAnnotation string
	DefaultNodeWeight    int
	nodeWeights          map[string]int
}

func NewNodeWeightProvider(fieldLogger *logrus.Entry) *NodeWeightProvider {
	return &NodeWeightProvider{
		FieldLogger: fieldLogger,
		nodeWeights: make(map[string]int),
	}
}

func (nwp *NodeWeightProvider) GetNodeWeight(nodeName *string) int {
	if nodeName != nil {
		if weight, ok := nwp.nodeWeights[*nodeName]; ok {
			return weight
		}
	}
	return nwp.DefaultNodeWeight
}

func (nwp *NodeWeightProvider) updateWeight(old *v1.Node, new *v1.Node) {
	if oldWeight, ok := nwp.nodeWeights[old.Name]; ok {
		newWeight := getWeightFromAnnotation(new.ObjectMeta, nwp.NodeWeightAnnotation, nwp.DefaultNodeWeight)
		if oldWeight != newWeight {
			nwp.nodeWeights[old.Name] = newWeight
		}
	}
}

func (nwp *NodeWeightProvider) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Node:
		nwp.nodeWeights[obj.Name] = getWeightFromAnnotation(obj.ObjectMeta, nwp.NodeWeightAnnotation, nwp.DefaultNodeWeight)
	default:
		nwp.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (nwp *NodeWeightProvider) OnUpdate(oldObj, newObj interface{}) {
	switch newObj := newObj.(type) {
	case *v1.Node:
		oldObj, ok := oldObj.(*v1.Node)
		if !ok {
			nwp.Errorf("OnUpdate node %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
		nwp.updateWeight(oldObj, newObj)
	default:
		nwp.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (nwp *NodeWeightProvider) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Node:
		delete(nwp.nodeWeights, obj.Name)
	case _cache.DeletedFinalStateUnknown:
		nwp.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		nwp.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func getWeightFromAnnotation(meta metav1.ObjectMeta, annotationName string, defaultWeight int) int {
	annotationWeight := getIntAnnotation(meta, annotationName, defaultWeight)
	return normalizeWeight(annotationWeight, defaultWeight)
}

func getIntAnnotation(meta metav1.ObjectMeta, name string, defaultValue int) int {
	annotationValue := defaultValue
	if annotationStringValue, ok := meta.Annotations[name]; ok {
		if nweight, cerr := strconv.ParseInt(annotationStringValue, 10, 32); cerr == nil {
			annotationValue = int(nweight)
		}
	}
	return annotationValue
}

func normalizeWeight(weight, defaultWeight int) int {
	if weight < 0 || weight > 128 {
		return defaultWeight
	}
	return weight
}
