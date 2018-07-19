package provider

import (
	"fmt"
	"regexp"

	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

// reMatcher either positively or negatively matches a regex
type reMatcher struct {
	regex    *regexp.Regexp
	positive bool
}

func newReMatcher(cfg config.RegexFilter) (*reMatcher, error) {
	if cfg.Is != "" && cfg.IsNot != "" {
		return nil, fmt.Errorf("cannot have both an `is` (%q) and `isNot` (%q) expression in a single filter", cfg.Is, cfg.IsNot)
	}
	if cfg.Is == "" && cfg.IsNot == "" {
		return nil, fmt.Errorf("must have either an `is` or `isNot` expression in a filter")
	}

	var positive bool
	var regexRaw string
	if cfg.Is != "" {
		positive = true
		regexRaw = cfg.Is
	} else {
		positive = false
		regexRaw = cfg.IsNot
	}

	regex, err := regexp.Compile(regexRaw)
	if err != nil {
		return nil, fmt.Errorf("unable to compile series filter %q: %v", regexRaw, err)
	}

	return &reMatcher{
		regex:    regex,
		positive: positive,
	}, nil
}

func (m *reMatcher) Matches(val string) bool {
	return m.regex.MatchString(val) == m.positive
}
