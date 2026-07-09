// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
)

// FieldExtractConfig is a single label/env extraction rule. Exactly one of Key
// or KeyRegex must be set. TagName is the target attribute name; when empty a
// default is derived from the source key. With KeyRegex, TagName may use back
// references ($1, $name). It lives here (not the root package) so ResolveExtract
// can consume it without an import cycle; the root package aliases it.
type FieldExtractConfig struct {
	Key      string `mapstructure:"key"`
	KeyRegex string `mapstructure:"key_regex"`
	TagName  string `mapstructure:"tag_name"`
}

// Validate reports whether the rule is well-formed.
func (f FieldExtractConfig) Validate() error {
	if f.Key != "" && f.KeyRegex != "" {
		return errors.New("only one of key or key_regex may be set")
	}
	if f.Key == "" && f.KeyRegex == "" {
		return errors.New("one of key or key_regex must be set")
	}
	if f.KeyRegex != "" {
		if _, err := regexp.Compile(f.KeyRegex); err != nil {
			return fmt.Errorf("invalid key_regex %q: %w", f.KeyRegex, err)
		}
	}
	return nil
}

const (
	labelAttrPrefix  = "container.label."
	envVarAttrPrefix = "container.env."
)

// ResolveExtract compiles label/env rules into the extractConfig that build
// reads. Regexes that fail to compile are skipped (validation rejects them
// first); the unexported return keeps the extractConfig contract internal.
//
//nolint:revive // unexported-return is deliberate
func ResolveExtract(labels, envVars []FieldExtractConfig) extractConfig {
	return extractConfig{
		labels:  buildFieldFilter(labels, labelAttrPrefix),
		envVars: buildFieldFilter(envVars, envVarAttrPrefix),
	}
}

// LabelsMap resolves a label key to its target attribute name. Exported so the
// root package can exercise the resolved rules without the unexported types.
func (c extractConfig) LabelsMap(key string) (string, bool) {
	if c.labels == nil {
		return "", false
	}
	return c.labels.Map(key)
}

// EnvVarsMap resolves an env var key to its target attribute name.
func (c extractConfig) EnvVarsMap(key string) (string, bool) {
	if c.envVars == nil {
		return "", false
	}
	return c.envVars.Map(key)
}

// buildFieldFilter combines the rules into one fieldFilter. defaultPrefix is
// prepended to the source key when a rule has no TagName.
func buildFieldFilter(rules []FieldExtractConfig, defaultPrefix string) fieldFilter {
	exact := exactFilter{}
	var regexes []regexFilter

	for _, r := range rules {
		switch {
		case r.KeyRegex != "":
			re, err := regexp.Compile(wrapAnchored(r.KeyRegex))
			if err != nil {
				continue
			}
			regexes = append(regexes, regexFilter{re: re, tagTmpl: r.TagName, defaultPrefix: defaultPrefix})
		case r.Key != "":
			name := r.TagName
			if name == "" {
				name = defaultPrefix + r.Key
			}
			exact[r.Key] = name
		}
	}

	switch {
	case len(exact) > 0 && len(regexes) > 0:
		return chainFilter{filters: []fieldFilter{exact, regexFilters(regexes)}}
	case len(regexes) > 0:
		return regexFilters(regexes)
	default:
		return exact
	}
}

// wrapAnchored anchors pattern to the whole key via a non-capturing group:
// `foo|bar` becomes `^(?:foo|bar)$`, not `^foo|bar$` (which would match
// `foo-secret`). Preserves back-reference semantics.
func wrapAnchored(pattern string) string {
	return "^(?:" + pattern + ")$"
}

// exactFilter maps a source key to a target attribute name by lookup.
type exactFilter map[string]string

func (f exactFilter) Map(key string) (string, bool) {
	name, ok := f[key]
	return name, ok
}

// regexFilter derives the attribute name from tagTmpl ($1/$name back refs), or
// defaultPrefix+key when tagTmpl is empty.
type regexFilter struct {
	re            *regexp.Regexp
	tagTmpl       string
	defaultPrefix string
}

func (f regexFilter) Map(key string) (string, bool) {
	m := f.re.FindStringSubmatchIndex(key)
	if m == nil {
		return "", false
	}
	if f.tagTmpl == "" {
		return f.defaultPrefix + key, true
	}
	return string(f.re.ExpandString(nil, f.tagTmpl, key, m)), true
}

// regexFilters returns the first matching regexFilter's result.
type regexFilters []regexFilter

func (fs regexFilters) Map(key string) (string, bool) {
	for _, f := range fs {
		if name, ok := f.Map(key); ok {
			return name, true
		}
	}
	return "", false
}

// chainFilter returns the first matching filter's result (exact rules before regex).
type chainFilter struct {
	filters []fieldFilter
}

func (c chainFilter) Map(key string) (string, bool) {
	for _, f := range c.filters {
		if name, ok := f.Map(key); ok {
			return name, true
		}
	}
	return "", false
}

// NewImageMatcher builds an image-exclusion matcher from a list of literals,
// globs, and /regex/ items, each optionally negated with a leading '!'.
func NewImageMatcher(excluded []string) (imageMatcher, error) {
	m := &imageExcludeMatcher{standardItems: map[string]bool{}}
	for _, raw := range excluded {
		item, isNegated := stripNegation(raw)
		switch {
		case isRegexItem(item):
			re, err := regexp.Compile(item[1 : len(item)-1])
			if err != nil {
				return nil, fmt.Errorf("invalid regex item %q: %w", raw, err)
			}
			m.regexItems = append(m.regexItems, negatedRegex{re: re, isNegated: isNegated})
		case isGlobItem(item):
			g, err := glob.Compile(item)
			if err != nil {
				return nil, fmt.Errorf("invalid glob item %q: %w", raw, err)
			}
			m.globItems = append(m.globItems, negatedGlob{glob: g, isNegated: isNegated})
		default:
			m.standardItems[item] = isNegated
			if isNegated {
				m.anyNegatedStandards = true
			}
		}
	}
	return m, nil
}

type imageExcludeMatcher struct {
	standardItems       map[string]bool
	anyNegatedStandards bool
	regexItems          []negatedRegex
	globItems           []negatedGlob
}

type negatedRegex struct {
	re        *regexp.Regexp
	isNegated bool
}

type negatedGlob struct {
	glob      glob.Glob
	isNegated bool
}

func (m *imageExcludeMatcher) Matches(image string) bool {
	if negated, ok := m.standardItems[image]; ok {
		return !negated
	}
	if m.anyNegatedStandards {
		return true
	}
	for _, r := range m.regexItems {
		if r.re.MatchString(image) != r.isNegated {
			return true
		}
	}
	for _, g := range m.globItems {
		if g.glob.Match(image) != g.isNegated {
			return true
		}
	}
	return false
}

func stripNegation(v string) (string, bool) {
	if strings.HasPrefix(v, "!") {
		return v[1:], true
	}
	return v, false
}

func isRegexItem(s string) bool {
	return len(s) > 2 && s[0] == '/' && s[len(s)-1] == '/'
}

func isGlobItem(s string) bool {
	return strings.ContainsAny(s, "*?[]{}!")
}
