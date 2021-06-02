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
	"testing"

	"github.com/stretchr/testify/require"

	"sigs.k8s.io/prometheus-adapter/pkg/config"
)

func TestReMatcherIs(t *testing.T) {
	filter := config.RegexFilter{
		Is: "my_.*",
	}

	matcher, err := NewReMatcher(filter)
	require.NoError(t, err)

	result := matcher.Matches("my_label")
	require.True(t, result)

	result = matcher.Matches("your_label")
	require.False(t, result)
}

func TestReMatcherIsNot(t *testing.T) {
	filter := config.RegexFilter{
		IsNot: "my_.*",
	}

	matcher, err := NewReMatcher(filter)
	require.NoError(t, err)

	result := matcher.Matches("my_label")
	require.False(t, result)

	result = matcher.Matches("your_label")
	require.True(t, result)
}

func TestEnforcesIsOrIsNotButNotBoth(t *testing.T) {
	filter := config.RegexFilter{
		Is:    "my_.*",
		IsNot: "your_.*",
	}

	_, err := NewReMatcher(filter)
	require.Error(t, err)
}
