// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	v1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

// PlacementDecisionLister helps list PlacementDecisions.
// All objects returned here must be treated as read-only.
type PlacementDecisionLister interface {
	// List lists all PlacementDecisions in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.PlacementDecision, err error)
	// PlacementDecisions returns an object that can list and get PlacementDecisions.
	PlacementDecisions(namespace string) PlacementDecisionNamespaceLister
	PlacementDecisionListerExpansion
}

// placementDecisionLister implements the PlacementDecisionLister interface.
type placementDecisionLister struct {
	indexer cache.Indexer
}

// NewPlacementDecisionLister returns a new PlacementDecisionLister.
func NewPlacementDecisionLister(indexer cache.Indexer) PlacementDecisionLister {
	return &placementDecisionLister{indexer: indexer}
}

// List lists all PlacementDecisions in the indexer.
func (s *placementDecisionLister) List(selector labels.Selector) (ret []*v1beta1.PlacementDecision, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.PlacementDecision))
	})
	return ret, err
}

// PlacementDecisions returns an object that can list and get PlacementDecisions.
func (s *placementDecisionLister) PlacementDecisions(namespace string) PlacementDecisionNamespaceLister {
	return placementDecisionNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// PlacementDecisionNamespaceLister helps list and get PlacementDecisions.
// All objects returned here must be treated as read-only.
type PlacementDecisionNamespaceLister interface {
	// List lists all PlacementDecisions in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.PlacementDecision, err error)
	// Get retrieves the PlacementDecision from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta1.PlacementDecision, error)
	PlacementDecisionNamespaceListerExpansion
}

// placementDecisionNamespaceLister implements the PlacementDecisionNamespaceLister
// interface.
type placementDecisionNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all PlacementDecisions in the indexer for a given namespace.
func (s placementDecisionNamespaceLister) List(selector labels.Selector) (ret []*v1beta1.PlacementDecision, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.PlacementDecision))
	})
	return ret, err
}

// Get retrieves the PlacementDecision from the indexer for a given namespace and name.
func (s placementDecisionNamespaceLister) Get(name string) (*v1beta1.PlacementDecision, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1beta1.Resource("placementdecision"), name)
	}
	return obj.(*v1beta1.PlacementDecision), nil
}