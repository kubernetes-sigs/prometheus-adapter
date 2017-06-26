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
	"bytes"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/pkg/api"

	"github.com/emicklei/go-restful"
)

type setTestSelfLinker struct {
	t           *testing.T
	expectedSet string
	name        string
	namespace   string
	called      bool
	err         error
}

func (s *setTestSelfLinker) Namespace(runtime.Object) (string, error) { return s.namespace, s.err }
func (s *setTestSelfLinker) Name(runtime.Object) (string, error)      { return s.name, s.err }
func (s *setTestSelfLinker) SelfLink(runtime.Object) (string, error)  { return "", s.err }
func (s *setTestSelfLinker) SetSelfLink(obj runtime.Object, selfLink string) error {
	if e, a := s.expectedSet, selfLink; e != a {
		s.t.Errorf("expected '%v', got '%v'", e, a)
	}
	s.called = true
	return s.err
}

func TestScopeNamingGenerateLink(t *testing.T) {
	selfLinker := &setTestSelfLinker{
		t:           t,
		expectedSet: "/api/v1/namespaces/other/services/foo",
		name:        "foo",
		namespace:   "other",
	}
	s := scopeNaming{
		meta.RESTScopeNamespace,
		selfLinker,
		func(name, namespace, resource, subresource string) bytes.Buffer {
			return *bytes.NewBufferString("/api/v1/namespaces/" + namespace + "/services/" + name)
		},
		true,
	}
	service := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "other",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
	}
	_, err := s.GenerateLink(&restful.Request{}, service)
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
}

