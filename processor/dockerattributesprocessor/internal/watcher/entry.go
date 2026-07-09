// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package watcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"

import (
	"strings"

	"github.com/docker/docker/api/types/container"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/open-telemetry/opentelemetry-collector-contrib/internal/common/docker"
)

// Attribute keys emitted onto resources. Derivation matches dockerstatsreceiver,
// not strict semconv.
const (
	attrContainerID          = "container.id"
	attrContainerName        = "container.name"
	attrContainerImageName   = "container.image.name"
	attrContainerImageID     = "container.image.id"
	attrContainerCommandLine = "container.command_line"
	attrContainerRuntimeName = "container.runtime.name"
	attrContainerImageTags   = "container.image.tags"

	runtimeDocker = "docker"
)

// entry is the pre-derived metadata for one container. Once stored in the cache
// it is never mutated; rename builds a new entry and swaps the map slot. That
// immutability is what lets cache reads skip copying.
type entry struct {
	id, name    string
	imageName   string
	imageID     string
	command     string
	runtimeName string
	imageTags   []string
	labels      map[string]string
	env         map[string]string
	stamp       int64 // event TimeNano; 0 for on-miss entries
}

// build derives an entry from an inspect response. ok is false when the image is
// excluded, in which case the container must not be cached.
func build(insp *container.InspectResponse, cfg extractConfig, excl imageMatcher) (e *entry, ok bool) {
	rawImage := ""
	if insp.Config != nil {
		rawImage = insp.Config.Image
	}
	if excl != nil && excl.Matches(rawImage) {
		return nil, false
	}

	e = &entry{
		id:          insp.ID,
		name:        strings.TrimPrefix(insp.Name, "/"),
		imageName:   rawImage,
		imageID:     insp.Image,
		runtimeName: runtimeDocker,
	}
	if insp.Config != nil {
		e.command = strings.Join(insp.Config.Cmd, " ")
		e.imageTags = deriveImageTags(rawImage)
		e.labels = filterMapped(insp.Config.Labels, cfg.labels)
		e.env = filterEnv(insp.Config.Env, cfg.envVars)
	}
	return e, true
}

// deriveImageTags returns the ref's tag, or nil when it carries none.
// ParseImageName defaults a missing tag to "latest", so we trust its tag only
// when the raw ref actually has one — never fabricating "latest".
func deriveImageTags(rawImage string) []string {
	if rawImage == "" || !hasExplicitTag(rawImage) {
		return nil
	}
	ref, err := docker.ParseImageName(rawImage)
	if err != nil || ref.Tag == "" {
		return nil
	}
	return []string{ref.Tag}
}

// hasExplicitTag reports whether the ref has a ":tag" section, distinguishing a
// tag from a registry port (host:5000/repo) by only looking after the last '/'.
func hasExplicitTag(rawImage string) bool {
	if at := strings.IndexByte(rawImage, '@'); at >= 0 {
		rawImage = rawImage[:at]
	}
	lastSlash := strings.LastIndexByte(rawImage, '/')
	return strings.IndexByte(rawImage[lastSlash+1:], ':') >= 0
}

// filterMapped keeps the source keys selected by f, under their mapped names.
// Returns nil when nothing matches.
func filterMapped(src map[string]string, f fieldFilter) map[string]string {
	if len(src) == 0 || f == nil {
		return nil
	}
	var out map[string]string
	for k, v := range src {
		attrName, ok := f.Map(k)
		if !ok {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[attrName] = v
	}
	return out
}

// filterEnv parses "KEY=VALUE" entries and keeps the selected keys. SplitN keeps
// '=' in values intact; empty-valued vars are kept.
func filterEnv(env []string, f fieldFilter) map[string]string {
	if len(env) == 0 || f == nil {
		return nil
	}
	var out map[string]string
	for _, v := range env {
		key, val, _ := strings.Cut(v, "=")
		attrName, ok := f.Map(key)
		if !ok {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[attrName] = val
	}
	return out
}

// writeTo copies the entry's attributes onto attrs without overwriting existing
// non-empty values.
func (e *entry) writeTo(attrs pcommon.Map) {
	putStr(attrs, attrContainerID, e.id)
	putStr(attrs, attrContainerName, e.name)
	putStr(attrs, attrContainerImageName, e.imageName)
	putStr(attrs, attrContainerImageID, e.imageID)
	putStr(attrs, attrContainerCommandLine, e.command)
	putStr(attrs, attrContainerRuntimeName, e.runtimeName)

	if len(e.imageTags) > 0 && !hasNonEmptyTags(attrs, attrContainerImageTags) {
		s := attrs.PutEmptySlice(attrContainerImageTags)
		for _, t := range e.imageTags {
			s.AppendEmpty().SetStr(t)
		}
	}

	// Labels/env are explicitly opted in, so an empty value is meaningful and
	// still written (unlike the fixed metadata above).
	for k, v := range e.labels {
		putStrAllowEmpty(attrs, k, v)
	}
	for k, v := range e.env {
		putStrAllowEmpty(attrs, k, v)
	}
}

// putStr sets key=val unless val is empty or key already holds a non-empty value.
func putStr(attrs pcommon.Map, key, val string) {
	if val == "" || hasNonEmpty(attrs, key) {
		return
	}
	attrs.PutStr(key, val)
}

// putStrAllowEmpty is putStr but stores an empty val too; still won't overwrite a
// non-empty value.
func putStrAllowEmpty(attrs pcommon.Map, key, val string) {
	if hasNonEmpty(attrs, key) {
		return
	}
	attrs.PutStr(key, val)
}

func hasNonEmpty(attrs pcommon.Map, key string) bool {
	v, ok := attrs.Get(key)
	return ok && v.Str() != ""
}

func hasNonEmptyTags(attrs pcommon.Map, key string) bool {
	v, ok := attrs.Get(key)
	return ok && v.Type() == pcommon.ValueTypeSlice && v.Slice().Len() > 0
}

// notActive reports whether the container is stopped or paused. A paused
// container has Running and Paused both true, so !Running alone is insufficient.
func notActive(s *container.State) bool {
	if s == nil {
		return true
	}
	return !s.Running || s.Paused
}
