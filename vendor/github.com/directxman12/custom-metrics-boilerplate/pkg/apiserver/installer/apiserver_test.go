/*
Copyright 2017 The Kubernetes Authors.

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

package installer

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emicklei/go-restful"

	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apimachinery/pkg/apimachinery"
	"k8s.io/metrics/pkg/apis/custom_metrics/install"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	cmv1alpha1 "k8s.io/metrics/pkg/apis/custom_metrics/v1alpha1"

	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"
	metricstorage "github.com/directxman12/custom-metrics-boilerplate/pkg/registry/custom_metrics"

)

// defaultAPIServer exposes nested objects for testability.
type defaultAPIServer struct {
	http.Handler
	container *restful.Container
}

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	Scheme               = runtime.NewScheme()
	Codecs               = serializer.NewCodecFactory(Scheme)
	prefix               = genericapiserver.APIGroupPrefix
	groupVersion         schema.GroupVersion
	groupMeta            *apimachinery.GroupMeta
	codec                = Codecs.LegacyCodec()
	emptySet             = labels.Set{}
	matchingSet          = labels.Set{"foo": "bar"}
)

func init() {
	install.Install(groupFactoryRegistry, registry, Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)

	groupMeta = registry.GroupOrDie(custom_metrics.GroupName)
	groupVersion = groupMeta.GroupVersion
}

func extractBody(response *http.Response, object runtime.Object) error {
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	return runtime.DecodeInto(codec, body, object)
}

func extractBodyString(response *http.Response) (string, error) {
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(body), err
}

func handle(prov provider.CustomMetricsProvider) http.Handler {
	container := restful.NewContainer()
	container.Router(restful.CurlyRouter{})
	mux := container.ServeMux
	resourceStorage := metricstorage.NewREST(prov)
	group := &MetricsAPIGroupVersion{
		DynamicStorage: resourceStorage,
		APIGroupVersion: &genericapi.APIGroupVersion{
			Root:         prefix,
			GroupVersion: groupVersion,

			ParameterCodec:  metav1.ParameterCodec,
			Serializer:      Codecs,
			Creater:         Scheme,
			Convertor:       Scheme,
			UnsafeConvertor: runtime.UnsafeObjectConvertor(Scheme),
			Copier:          Scheme,
			Typer:           Scheme,
			Linker:          groupMeta.SelfLinker,
			Mapper:          groupMeta.RESTMapper,

			Context:                request.NewRequestContextMapper(),
			OptionsExternalVersion: &schema.GroupVersion{Version: "v1"},

			ResourceLister: provider.NewResourceLister(prov),
		},
	}

	if err := group.InstallREST(container); err != nil {
		panic(fmt.Sprintf("unable to install container %s: %v", group.GroupVersion, err))
	}

	return &defaultAPIServer{mux, container}
}

type fakeProvider struct {
	rootValues             map[string][]custom_metrics.MetricValue
	namespacedValues       map[string][]custom_metrics.MetricValue
	rootSubsetCounts       map[string]int
	namespacedSubsetCounts map[string]int
	metrics                []provider.MetricInfo
}

func (p *fakeProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	metricId := groupResource.String()+"/"+name+"/"+metricName
	values, ok := p.rootValues[metricId]
	if !ok {
		return nil, fmt.Errorf("non-existant metric requested (id: %s)", metricId)
	}

	return &values[0], nil
}

func (p *fakeProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	metricId := groupResource.String()+"/*/"+metricName
	values, ok := p.rootValues[metricId]
	if !ok {
		return nil, fmt.Errorf("non-existant metric requested (id: %s)", metricId)
	}

	var trimmedValues custom_metrics.MetricValueList

	if trimmedCount, ok := p.rootSubsetCounts[metricId]; ok {
		trimmedValues = custom_metrics.MetricValueList{
			Items: make([]custom_metrics.MetricValue, 0, trimmedCount),
		}
		for i := range values {
			var lbls labels.Labels
			if i < trimmedCount {
				lbls = matchingSet
			} else {
				lbls = emptySet
			}
			if selector.Matches(lbls) {
				trimmedValues.Items = append(trimmedValues.Items, custom_metrics.MetricValue{})
			}
		}
	} else {
		trimmedValues = custom_metrics.MetricValueList{
			Items: values,
		}
	}

	return &trimmedValues, nil
}

func (p *fakeProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	metricId := namespace+"/"+groupResource.String()+"/"+name+"/"+metricName
	values, ok := p.namespacedValues[metricId]
	if !ok {
		return nil, fmt.Errorf("non-existant metric requested (id: %s)", metricId)
	}

	return &values[0], nil
}

func (p *fakeProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	metricId := namespace+"/"+groupResource.String()+"/*/"+metricName
	values, ok := p.namespacedValues[metricId]
	if !ok {
		return nil, fmt.Errorf("non-existant metric requested (id: %s)", metricId)
	}

	var trimmedValues custom_metrics.MetricValueList

	if trimmedCount, ok := p.namespacedSubsetCounts[metricId]; ok {
		trimmedValues = custom_metrics.MetricValueList{
			Items: make([]custom_metrics.MetricValue, 0, trimmedCount),
		}
		for i := range values {
			var lbls labels.Labels
			if i < trimmedCount {
				lbls = matchingSet
			} else {
				lbls = emptySet
			}
			if selector.Matches(lbls) {
				trimmedValues.Items = append(trimmedValues.Items, custom_metrics.MetricValue{})
			}
		}
	} else {
		trimmedValues = custom_metrics.MetricValueList{
			Items: values,
		}
	}

	return &trimmedValues, nil
}

func (p *fakeProvider) ListAllMetrics() []provider.MetricInfo {
	return p.metrics
}

func TestCustomMetricsAPI(t *testing.T) {
	totalNodesCount := 4
	totalPodsCount := 16
	matchingNodesCount := 2
	matchingPodsCount := 8

	type T struct {
		Method        string
		Path          string
		Status        int
		ExpectedCount int
	}
	cases := map[string]T{
		// checks which should fail
		"GET long prefix": {"GET", "/" + prefix + "/", http.StatusNotFound, 0},

		"root GET missing storage":    {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/blah", http.StatusNotFound, 0},

		"namespaced GET long prefix":        {"GET", "/" + prefix + "/", http.StatusNotFound, 0},
		"namespaced GET missing storage":    {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/blah", http.StatusNotFound, 0},

		"GET at root resource leaf":        {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/nodes/foo", http.StatusNotFound, 0},
		"GET at namespaced resource leaft": {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/namespaces/ns/pods/bar", http.StatusNotFound, 0},

		// Positive checks to make sure everything is wired correctly
		"GET for all nodes (root)":                 {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/nodes/*/some-metric", http.StatusOK, totalNodesCount},
		"GET for all pods (namespaced)":            {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/namespaces/ns/pods/*/some-metric", http.StatusOK, totalPodsCount},
		"GET for namespace":                        {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/namespaces/ns/metrics/some-metric", http.StatusOK, 1},
		"GET for label selected nodes (root)":      {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/nodes/*/some-metric?labelSelector=foo%3Dbar", http.StatusOK, matchingNodesCount},
		"GET for label selected pods (namespaced)": {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/namespaces/ns/pods/*/some-metric?labelSelector=foo%3Dbar", http.StatusOK, matchingPodsCount},
		"GET for single node (root)":               {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/nodes/foo/some-metric", http.StatusOK, 1},
		"GET for single pod (namespaced)":          {"GET", "/" + prefix + "/" + groupVersion.Group + "/" + groupVersion.Version + "/namespaces/ns/pods/foo/some-metric", http.StatusOK, 1},
	}

	prov := &fakeProvider{
		rootValues: map[string][]custom_metrics.MetricValue{
			"nodes/*/some-metric":   make([]custom_metrics.MetricValue, totalNodesCount),
			"nodes/foo/some-metric": make([]custom_metrics.MetricValue, 1),
			"namespaces/ns/some-metric": make([]custom_metrics.MetricValue, 1),
		},
		namespacedValues: map[string][]custom_metrics.MetricValue{
			"ns/pods/*/some-metric":   make([]custom_metrics.MetricValue, totalPodsCount),
			"ns/pods/foo/some-metric": make([]custom_metrics.MetricValue, 1),
		},

		rootSubsetCounts: map[string]int{
			"nodes/*/some-metric": matchingNodesCount,
		},
		namespacedSubsetCounts: map[string]int{
			"ns/pods/*/some-metric": matchingPodsCount,
		},
	}

	server := httptest.NewServer(handle(prov))
	defer server.Close()
	client := http.Client{}
	for k, v := range cases {
		request, err := http.NewRequest(v.Method, server.URL+v.Path, nil)
		if err != nil {
			t.Fatalf("unexpected error (%s): %v", k, err)
		}

		response, err := client.Do(request)
		if err != nil {
			t.Errorf("unexpected error (%s): %v", k, err)
			continue
		}

		if response.StatusCode != v.Status {
			body, err := extractBodyString(response)
			bodyPart := body
			if err != nil {
				bodyPart = fmt.Sprintf("[error extracting body: %v]", err)
			}
			t.Errorf("Expected %d for %s (%s), Got %#v -- %s", v.Status, v.Method, k, response, bodyPart)
			continue
		}

		if v.ExpectedCount > 0 {
			lst := &cmv1alpha1.MetricValueList{}
			if err := extractBody(response, lst); err != nil {
				t.Errorf("unexpected error (%s): %v", k, err)
				continue
			}
			if len(lst.Items) != v.ExpectedCount {
				t.Errorf("Expected %d items, got %d (%s): %#v", v.ExpectedCount, len(lst.Items), k, lst.Items)
				continue
			}
		}
	}
}
