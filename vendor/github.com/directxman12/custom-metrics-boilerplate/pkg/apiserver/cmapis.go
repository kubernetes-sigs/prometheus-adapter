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

package apiserver

import (
	"k8s.io/apimachinery/pkg/apimachinery"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/endpoints/discovery"

	specificapi "github.com/directxman12/custom-metrics-boilerplate/pkg/apiserver/installer"
	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"
	metricstorage "github.com/directxman12/custom-metrics-boilerplate/pkg/registry/custom_metrics"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

func (s *CustomMetricsAdapterServer) InstallCustomMetricsAPI() error {

	groupMeta := registry.GroupOrDie(custom_metrics.GroupName)

	preferredVersionForDiscovery := metav1.GroupVersionForDiscovery{
		GroupVersion: groupMeta.GroupVersion.String(),
		Version:      groupMeta.GroupVersion.Version,
	}
	groupVersion := metav1.GroupVersionForDiscovery{
		GroupVersion: groupMeta.GroupVersion.String(),
		Version:      groupMeta.GroupVersion.Version,
	}
	apiGroup := metav1.APIGroup{
		Name:             groupMeta.GroupVersion.Group,
		Versions:         []metav1.GroupVersionForDiscovery{groupVersion},
		PreferredVersion: preferredVersionForDiscovery,
	}

	cmAPI := s.cmAPI(groupMeta, &groupMeta.GroupVersion)

	if err := cmAPI.InstallREST(s.GenericAPIServer.Handler.GoRestfulContainer); err != nil {
		return err
	}

	s.GenericAPIServer.DiscoveryGroupManager.AddGroup(apiGroup)
	s.GenericAPIServer.Handler.GoRestfulContainer.Add(discovery.NewAPIGroupHandler(s.GenericAPIServer.Serializer, apiGroup).WebService())

	return nil
}
func (s *CustomMetricsAdapterServer) cmAPI(groupMeta *apimachinery.GroupMeta, groupVersion *schema.GroupVersion) *specificapi.MetricsAPIGroupVersion {
	resourceStorage := metricstorage.NewREST(s.Provider)

	return &specificapi.MetricsAPIGroupVersion{
		DynamicStorage: resourceStorage,
		APIGroupVersion: &genericapi.APIGroupVersion{
			Root:         genericapiserver.APIGroupPrefix,
			GroupVersion: *groupVersion,

			ParameterCodec:  metav1.ParameterCodec,
			Serializer:      Codecs,
			Creater:         Scheme,
			Convertor:       Scheme,
			UnsafeConvertor: runtime.UnsafeObjectConvertor(Scheme),
			Copier:          Scheme,
			Typer:           Scheme,
			Linker:          groupMeta.SelfLinker,
			Mapper:          groupMeta.RESTMapper,

			Context:                s.GenericAPIServer.RequestContextMapper(),
			MinRequestTimeout:      s.GenericAPIServer.MinRequestTimeout(),
			OptionsExternalVersion: &schema.GroupVersion{Version: "v1"},

			ResourceLister: provider.NewResourceLister(s.Provider),
		},
	}
}
