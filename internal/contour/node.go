package contour

import (
	"strconv"

	logrus "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_cache "k8s.io/client-go/tools/cache"
)

type NodeWeightProvider interface {
	GetNodeWeight(nodeName *string) int
	RegisterOnNodeWeightsChanged(func())
}

type NodeWeightCache struct {
	NodeWeightProvider
	logrus.FieldLogger
	NodeWeightAnnotation      string
	DefaultNodeWeight         int
	nodeWeights               map[string]int
	nodeWeightsChangedHandler func()
}

func NewNodeWeightProvider(fieldLogger logrus.FieldLogger) NodeWeightProvider {
	return &NodeWeightCache{
		FieldLogger: fieldLogger,
		nodeWeights: make(map[string]int),
	}
}

func (nwp *NodeWeightCache) GetNodeWeight(nodeName *string) int {
	if nodeName != nil {
		if weight, ok := nwp.nodeWeights[*nodeName]; ok {
			return weight
		}
	}
	return nwp.DefaultNodeWeight
}

func (nwp *NodeWeightCache) RegisterOnNodeWeightsChanged(handler func()) {
	nwp.nodeWeightsChangedHandler = handler
}

func (nwp *NodeWeightCache) updateWeight(old, new *v1.Node) {
	if oldWeight, ok := nwp.nodeWeights[old.Name]; ok {
		newWeight := getWeightFromAnnotation(new.ObjectMeta, nwp.NodeWeightAnnotation, nwp.DefaultNodeWeight)
		if oldWeight != newWeight {
			nwp.nodeWeights[old.Name] = newWeight
			nwp.fireNodeWeightsChanged()
		}
	}
}

func (nwp *NodeWeightCache) setWeight(node *v1.Node) {
	weight, ok := nwp.nodeWeights[node.Name]
	newWeight := getWeightFromAnnotation(node.ObjectMeta, nwp.NodeWeightAnnotation, nwp.DefaultNodeWeight)

	if !ok || weight != newWeight {
		nwp.nodeWeights[node.Name] = newWeight
		nwp.fireNodeWeightsChanged()
	}
}

func (nwp *NodeWeightCache) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Node:
		nwp.setWeight(obj)
	default:
		nwp.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (nwp *NodeWeightCache) OnUpdate(oldObj, newObj interface{}) {
	switch newObj := newObj.(type) {
	case *v1.Node:
		oldObj, ok := oldObj.(*v1.Node)
		if !ok {
			nwp.Errorf("OnUpdate node %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
		nwp.updateWeight(oldObj, newObj)
	case *v1.Endpoints:
		oldObj, ok := oldObj.(*v1.Endpoints)
		if !ok {
			nwp.Errorf("OnUpdate endpoints %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}
	default:
		nwp.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (nwp *NodeWeightCache) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Node:
		delete(nwp.nodeWeights, obj.Name)
	case _cache.DeletedFinalStateUnknown:
		nwp.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		nwp.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

func (nwp *NodeWeightCache) fireNodeWeightsChanged() {
	if nwp.nodeWeightsChangedHandler != nil {
		nwp.nodeWeightsChangedHandler()
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
