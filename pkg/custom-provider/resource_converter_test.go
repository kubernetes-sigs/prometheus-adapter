package provider

import (
	"testing"

	"github.com/prometheus/common/model"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

func TestCanCreateLabelFromResource(t *testing.T) {
	mapper := restMapper()
	converter, err := NewResourceConverter("kube_<<.Group>>_<<.Resource>>", map[string]config.GroupResource{}, mapper)
	require.NoError(t, err)

	result, err := converter.LabelForResource(schema.GroupResource{
		Group:    "apps",
		Resource: "deployment",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, model.LabelName("kube_apps_deployment"), result)
}

func TestDetectsResourcesFromLabel(t *testing.T) {
	mapper := restMapper()
	converter, err := NewResourceConverter("kube_<<.Group>>_<<.Resource>>", map[string]config.GroupResource{}, mapper)
	require.NoError(t, err)

	resource, namespaced := converter.ResourcesForSeries(prom.Series{
		Name: "some_series",
		Labels: model.LabelSet{
			"kube_extensions_deployment": "my_deployment",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []schema.GroupResource{schema.GroupResource{
		Group:    "extensions",
		Resource: "deployments",
	}}, resource)
	require.Equal(t, false, namespaced)
}

func TestDetectsNamespacedFromOverrides(t *testing.T) {
	mapper := restMapper()
	converter, err := NewResourceConverter("kube_<<.Group>>_<<.Resource>>", map[string]config.GroupResource{
		"has_namespace": config.GroupResource{
			Group:    "",
			Resource: "Namespaces",
		},
	}, mapper)
	require.NoError(t, err)

	resource, namespaced := converter.ResourcesForSeries(prom.Series{
		Name: "some_series",
		Labels: model.LabelSet{
			"kube_extensions_deployment": "my_deployment",
			"has_namespace":              "some_namespace",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []schema.GroupResource{
		schema.GroupResource{
			Group:    "extensions",
			Resource: "deployments",
		},
		schema.GroupResource{
			Group:    "",
			Resource: "namespaces",
		}}, resource)
	require.Equal(t, true, namespaced)
}

func TestDetectsNonNamespaceResourcesFromOverrides(t *testing.T) {
	mapper := restMapper()
	converter, err := NewResourceConverter("kube_<<.Group>>_<<.Resource>>", map[string]config.GroupResource{
		"a_special_label": config.GroupResource{
			Group:    "",
			Resource: "pod",
		},
	}, mapper)
	require.NoError(t, err)

	resource, namespaced := converter.ResourcesForSeries(prom.Series{
		Name: "some_series",
		Labels: model.LabelSet{
			"kube_extensions_deployment": "my_deployment",
			"a_special_label":            "a_special_value",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []schema.GroupResource{
		schema.GroupResource{
			Group:    "extensions",
			Resource: "deployments",
		},
		schema.GroupResource{
			Group:    "",
			Resource: "pods",
		},
	}, resource)
	require.Equal(t, false, namespaced)
}
