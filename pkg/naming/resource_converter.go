/*
Copyright 2019 The Kubernetes Authors.

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

package naming

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"

	pmodel "github.com/prometheus/common/model"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/config"
)

var (
	GroupNameSanitizer = strings.NewReplacer(".", "_", "-", "_")
	NsGroupResource    = schema.GroupResource{Resource: "namespaces"}
	NodeGroupResource  = schema.GroupResource{Resource: "nodes"}
	PVGroupResource    = schema.GroupResource{Resource: "persistentvolumes"}
)

// ResourceConverter knows the relationship between Kubernetes group-resources and Prometheus labels,
// and can convert between the two for any given label or series.
type ResourceConverter interface {
	// ResourcesForSeries returns the group-resources associated with the given series,
	// as well as whether or not the given series has the "namespace" resource).
	ResourcesForSeries(series prom.Series) (res []schema.GroupResource, namespaced bool)
	// LabelForResource returns the appropriate label for the given resource.
	LabelForResource(resource schema.GroupResource) (pmodel.LabelName, error)
}

type resourceConverter struct {
	labelResourceMu   sync.RWMutex
	labelToResource   map[pmodel.LabelName]schema.GroupResource
	resourceToLabel   map[schema.GroupResource]pmodel.LabelName
	labelResExtractor *labelGroupResExtractor
	mapper            apimeta.RESTMapper
	labelTemplate     *template.Template
}

// NewResourceConverter creates a ResourceConverter based on a generic template plus any overrides.
// Either overrides or the template may be empty, but not both.
func NewResourceConverter(resourceTemplate string, overrides map[string]config.GroupResource, mapper apimeta.RESTMapper) (ResourceConverter, error) {
	converter := &resourceConverter{
		labelToResource: make(map[pmodel.LabelName]schema.GroupResource),
		resourceToLabel: make(map[schema.GroupResource]pmodel.LabelName),
		mapper:          mapper,
	}

	if resourceTemplate != "" {
		labelTemplate, err := template.New("resource-label").Delims("<<", ">>").Parse(resourceTemplate)
		if err != nil {
			return converter, fmt.Errorf("unable to parse label template %q: %v", resourceTemplate, err)
		}
		converter.labelTemplate = labelTemplate

		labelResExtractor, err := newLabelGroupResExtractor(labelTemplate)
		if err != nil {
			return converter, fmt.Errorf("unable to generate label format from template %q: %v", resourceTemplate, err)
		}
		converter.labelResExtractor = labelResExtractor
	}

	// invert the structure for consistency with the template
	for lbl, groupRes := range overrides {
		infoRaw := provider.CustomMetricInfo{
			GroupResource: schema.GroupResource{
				Group:    groupRes.Group,
				Resource: groupRes.Resource,
			},
		}
		info, _, err := infoRaw.Normalized(converter.mapper)
		if err != nil {
			return nil, fmt.Errorf("unable to normalize group-resource %v: %v", groupRes, err)
		}

		converter.labelToResource[pmodel.LabelName(lbl)] = info.GroupResource
		converter.resourceToLabel[info.GroupResource] = pmodel.LabelName(lbl)
	}

	return converter, nil
}

func (r *resourceConverter) LabelForResource(resource schema.GroupResource) (pmodel.LabelName, error) {
	r.labelResourceMu.RLock()
	// check if we have a cached copy or override
	lbl, ok := r.resourceToLabel[resource]
	r.labelResourceMu.RUnlock() // release before we call makeLabelForResource
	if ok {
		return lbl, nil
	}

	// NB: we don't actually care about the gap between releasing read lock
	// and acquiring the write lock -- if we do duplicate work sometimes, so be
	// it, as long as we're correct.

	// otherwise, use the template and save the result
	lbl, err := r.makeLabelForResource(resource)
	if err != nil {
		return "", fmt.Errorf("unable to convert resource %s into label: %v", resource.String(), err)
	}
	return lbl, nil
}

// makeLabelForResource constructs a label name for the given resource, and saves the result.
// It must *not* be called under an existing lock.
func (r *resourceConverter) makeLabelForResource(resource schema.GroupResource) (pmodel.LabelName, error) {
	if r.labelTemplate == nil {
		return "", fmt.Errorf("no generic resource label form specified for this metric")
	}
	buff := new(bytes.Buffer)

	singularRes, err := r.mapper.ResourceSingularizer(resource.Resource)
	if err != nil {
		return "", fmt.Errorf("unable to singularize resource %s: %v", resource.String(), err)
	}
	convResource := schema.GroupResource{
		Group:    GroupNameSanitizer.Replace(resource.Group),
		Resource: singularRes,
	}

	if err := r.labelTemplate.Execute(buff, convResource); err != nil {
		return "", err
	}
	if buff.Len() == 0 {
		return "", fmt.Errorf("empty label produced by label template")
	}
	lbl := pmodel.LabelName(buff.String())

	r.labelResourceMu.Lock()
	defer r.labelResourceMu.Unlock()

	r.resourceToLabel[resource] = lbl
	r.labelToResource[lbl] = resource
	return lbl, nil
}

func (r *resourceConverter) ResourcesForSeries(series prom.Series) ([]schema.GroupResource, bool) {
	// use an updates map to avoid having to drop the read lock to update the cache
	// until the end.  Since we'll probably have few updates after the first run,
	// this should mean that we rarely have to hold the write lock.
	var resources []schema.GroupResource
	updates := make(map[pmodel.LabelName]schema.GroupResource)
	namespaced := false

	// use an anon func to get the right defer behavior
	func() {
		r.labelResourceMu.RLock()
		defer r.labelResourceMu.RUnlock()

		for lbl := range series.Labels {
			var groupRes schema.GroupResource
			var ok bool

			// check if we have an override
			if groupRes, ok = r.labelToResource[lbl]; ok {
				resources = append(resources, groupRes)
			} else if groupRes, ok = updates[lbl]; ok {
				resources = append(resources, groupRes)
			} else if r.labelResExtractor != nil {
				// if not, check if it matches the form we expect, and if so,
				// convert to a group-resource.
				if groupRes, ok = r.labelResExtractor.GroupResourceForLabel(lbl); ok {
					info, _, err := provider.CustomMetricInfo{GroupResource: groupRes}.Normalized(r.mapper)
					if err != nil {
						// this is likely to show up for a lot of labels, so make it a verbose info log
						klog.V(9).Infof("unable to normalize group-resource %s from label %q, skipping: %v", groupRes.String(), lbl, err)
						continue
					}

					groupRes = info.GroupResource
					resources = append(resources, groupRes)
					updates[lbl] = groupRes
				}
			}

			if groupRes != NsGroupResource && groupRes != NodeGroupResource && groupRes != PVGroupResource {
				namespaced = true
			}
		}
	}()

	// update the cache for next time.  This should only be called by discovery,
	// so we don't really have to worry about the gap between read and write locks
	// (plus, we don't care if someone else updates the cache first, since the results
	// are necessarily the same, so at most we've done extra work).
	if len(updates) > 0 {
		r.labelResourceMu.Lock()
		defer r.labelResourceMu.Unlock()

		for lbl, groupRes := range updates {
			r.labelToResource[lbl] = groupRes
		}
	}

	return resources, namespaced
}
