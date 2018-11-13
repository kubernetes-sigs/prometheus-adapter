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
package client

import (
	"fmt"
	"strings"
)

// LabelNeq produces a not-equal label selector expression.
// Label is passed verbatim, and value is double-quote escaped
// using Go's escaping (as per the PromQL rules).
func LabelNeq(label string, value string) string {
	return fmt.Sprintf("%s!=%q", label, value)
}

// LabelEq produces a equal label selector expression.
// Label is passed verbatim, and value is double-quote escaped
// using Go's escaping (as per the PromQL rules).
func LabelEq(label string, value string) string {
	return fmt.Sprintf("%s=%q", label, value)
}

// LabelMatches produces a regexp-matching label selector expression.
// It has similar constraints to LabelNeq.
func LabelMatches(label string, expr string) string {
	return fmt.Sprintf("%s=~%q", label, expr)
}

// LabelNotMatches produces a inverse regexp-matching label selector expression (the opposite of LabelMatches).
func LabelNotMatches(label string, expr string) string {
	return fmt.Sprintf("%s!~%q", label, expr)
}

// NameMatches produces a label selector expression that checks that the series name matches the given expression.
// It's a convinience wrapper around LabelMatches.
func NameMatches(expr string) string {
	return LabelMatches("__name__", expr)
}

// NameNotMatches produces a label selector expression that checks that the series name doesn't matches the given expression.
// It's a convenience wrapper around LabelNotMatches.
func NameNotMatches(expr string) string {
	return LabelNotMatches("__name__", expr)
}

// MatchSeries takes a series name, and optionally some label expressions, and returns a series selector.
// TODO: validate series name and expressions?
func MatchSeries(name string, labelExpressions ...string) Selector {
	if len(labelExpressions) == 0 {
		return Selector(name)
	}

	return Selector(fmt.Sprintf("%s{%s}", name, strings.Join(labelExpressions, ",")))
}
