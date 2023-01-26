//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The KCP Authors.

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

// Code generated by kcp code-generator. DO NOT EDIT.

package v1alpha1

import (
	kcpcache "github.com/kcp-dev/apimachinery/v2/pkg/cache"
	"github.com/kcp-dev/logicalcluster/v3"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
)

// APIResourceSchemaClusterLister can list APIResourceSchemas across all workspaces, or scope down to a APIResourceSchemaLister for one workspace.
// All objects returned here must be treated as read-only.
type APIResourceSchemaClusterLister interface {
	// List lists all APIResourceSchemas in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*apisv1alpha1.APIResourceSchema, err error)
	// Cluster returns a lister that can list and get APIResourceSchemas in one workspace.
	Cluster(clusterName logicalcluster.Name) APIResourceSchemaLister
	APIResourceSchemaClusterListerExpansion
}

type aPIResourceSchemaClusterLister struct {
	indexer cache.Indexer
}

// NewAPIResourceSchemaClusterLister returns a new APIResourceSchemaClusterLister.
// We assume that the indexer:
// - is fed by a cross-workspace LIST+WATCH
// - uses kcpcache.MetaClusterNamespaceKeyFunc as the key function
// - has the kcpcache.ClusterIndex as an index
func NewAPIResourceSchemaClusterLister(indexer cache.Indexer) *aPIResourceSchemaClusterLister {
	return &aPIResourceSchemaClusterLister{indexer: indexer}
}

// List lists all APIResourceSchemas in the indexer across all workspaces.
func (s *aPIResourceSchemaClusterLister) List(selector labels.Selector) (ret []*apisv1alpha1.APIResourceSchema, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*apisv1alpha1.APIResourceSchema))
	})
	return ret, err
}

// Cluster scopes the lister to one workspace, allowing users to list and get APIResourceSchemas.
func (s *aPIResourceSchemaClusterLister) Cluster(clusterName logicalcluster.Name) APIResourceSchemaLister {
	return &aPIResourceSchemaLister{indexer: s.indexer, clusterName: clusterName}
}

// APIResourceSchemaLister can list all APIResourceSchemas, or get one in particular.
// All objects returned here must be treated as read-only.
type APIResourceSchemaLister interface {
	// List lists all APIResourceSchemas in the workspace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*apisv1alpha1.APIResourceSchema, err error)
	// Get retrieves the APIResourceSchema from the indexer for a given workspace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*apisv1alpha1.APIResourceSchema, error)
	APIResourceSchemaListerExpansion
}

// aPIResourceSchemaLister can list all APIResourceSchemas inside a workspace.
type aPIResourceSchemaLister struct {
	indexer     cache.Indexer
	clusterName logicalcluster.Name
}

// List lists all APIResourceSchemas in the indexer for a workspace.
func (s *aPIResourceSchemaLister) List(selector labels.Selector) (ret []*apisv1alpha1.APIResourceSchema, err error) {
	err = kcpcache.ListAllByCluster(s.indexer, s.clusterName, selector, func(i interface{}) {
		ret = append(ret, i.(*apisv1alpha1.APIResourceSchema))
	})
	return ret, err
}

// Get retrieves the APIResourceSchema from the indexer for a given workspace and name.
func (s *aPIResourceSchemaLister) Get(name string) (*apisv1alpha1.APIResourceSchema, error) {
	key := kcpcache.ToClusterAwareKey(s.clusterName.String(), "", name)
	obj, exists, err := s.indexer.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(apisv1alpha1.Resource("apiresourceschemas"), name)
	}
	return obj.(*apisv1alpha1.APIResourceSchema), nil
}

// NewAPIResourceSchemaLister returns a new APIResourceSchemaLister.
// We assume that the indexer:
// - is fed by a workspace-scoped LIST+WATCH
// - uses cache.MetaNamespaceKeyFunc as the key function
func NewAPIResourceSchemaLister(indexer cache.Indexer) *aPIResourceSchemaScopedLister {
	return &aPIResourceSchemaScopedLister{indexer: indexer}
}

// aPIResourceSchemaScopedLister can list all APIResourceSchemas inside a workspace.
type aPIResourceSchemaScopedLister struct {
	indexer cache.Indexer
}

// List lists all APIResourceSchemas in the indexer for a workspace.
func (s *aPIResourceSchemaScopedLister) List(selector labels.Selector) (ret []*apisv1alpha1.APIResourceSchema, err error) {
	err = cache.ListAll(s.indexer, selector, func(i interface{}) {
		ret = append(ret, i.(*apisv1alpha1.APIResourceSchema))
	})
	return ret, err
}

// Get retrieves the APIResourceSchema from the indexer for a given workspace and name.
func (s *aPIResourceSchemaScopedLister) Get(name string) (*apisv1alpha1.APIResourceSchema, error) {
	key := name
	obj, exists, err := s.indexer.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(apisv1alpha1.Resource("apiresourceschemas"), name)
	}
	return obj.(*apisv1alpha1.APIResourceSchema), nil
}
