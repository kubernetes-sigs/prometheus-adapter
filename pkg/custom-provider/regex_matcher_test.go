package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

func TestReMatcherIs(t *testing.T) {
	filter := config.RegexFilter{
		Is: "my_.*",
	}

	matcher, err := newReMatcher(filter)
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

	matcher, err := newReMatcher(filter)
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

	_, err := newReMatcher(filter)
	require.Error(t, err)
}
