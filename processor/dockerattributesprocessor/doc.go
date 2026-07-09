// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

// Package dockerattributesprocessor enriches traces, metrics, and logs with
// Docker container metadata, keyed by the container.id resource attribute.
package dockerattributesprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor"
