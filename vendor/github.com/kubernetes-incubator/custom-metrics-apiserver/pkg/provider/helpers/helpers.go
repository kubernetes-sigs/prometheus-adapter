/*
Copyright 2018 The Kubernetes Authors.

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

package helpers

import (
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
)

// ResourceFor attempts to resolve a single qualified resource for the given metric.
// You can use this to resolve a particular piece of CustomMetricInfo to the underlying
// resource that it describes, so that you can list matching objects in the cluster.
func ResourceFor(mapper apimeta.RESTMapper, info provider.CustomMetricInfo) (schema.GroupVersionResource, error) {
	fullResources, err := mapper.ResourcesFor(info.GroupResource.WithVersion(""))
	if err == nil && len(fullResources) == 0 {
		err = fmt.Errorf("no fully versioned resources known for group-resource %v", info.GroupResource)
	}
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("unable to find preferred version to list matching resource names: %v", err)
	}

	return fullResources[0], nil
}

// ReferenceFor returns a new ObjectReference for the given group-resource and name.
// The group-resource is converted into a group-version-kind using the given RESTMapper.
// You can use this to easily construct an object reference for use in the DescribedObject
// field of CustomMetricInfo.
func ReferenceFor(mapper apimeta.RESTMapper, name types.NamespacedName, info provider.CustomMetricInfo) (custom_metrics.ObjectReference, error) {
	kind, err := mapper.KindFor(info.GroupResource.WithVersion(""))
	if err != nil {
		return custom_metrics.ObjectReference{}, err
	}

	// NB: return straight value, not a reference, so that the object can easily
	// be copied for use multiple times with a different name.
	return custom_metrics.ObjectReference{
		APIVersion: kind.Group + "/" + kind.Version,
		Kind:       kind.Kind,
		Name:       name.Name,
		Namespace:  name.Namespace,
	}, nil
}

// ListObjectNames uses the given dynamic client to list the names of all objects
// of the given resource matching the given selector.  Namespace may be empty
// if the metric is for a root-scoped resource.
func ListObjectNames(mapper apimeta.RESTMapper, client dynamic.Interface, namespace string, selector labels.Selector, info provider.CustomMetricInfo) ([]string, error) {
	res, err := ResourceFor(mapper, info)
	if err != nil {
		return nil, err
	}

	var resClient dynamic.ResourceInterface
	if info.Namespaced {
		resClient = client.Resource(res).Namespace(namespace)
	} else {
		resClient = client.Resource(res)
	}

	matchingObjectsRaw, err := resClient.List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}

	if !apimeta.IsListType(matchingObjectsRaw) {
		return nil, fmt.Errorf("result of label selector list operation was not a list")
	}

	var names []string
	err = apimeta.EachListItem(matchingObjectsRaw, func(item runtime.Object) error {
		objName := item.(*unstructured.Unstructured).GetName()
		names = append(names, objName)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return names, nil
}
