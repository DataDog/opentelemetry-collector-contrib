// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package dockerattributesprocessor // import "github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor"

import (
	"context"
	"encoding/hex"
	"strings"

	docker "github.com/docker/docker/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/dockerattributesprocessor/internal/watcher"
)

const processorUserAgent = "otelcol-docker-attributes-processor"

// dockerAttributesProcessor enriches telemetry with Docker container metadata.
type dockerAttributesProcessor struct {
	logger  *zap.Logger
	cfg     *Config
	sdk     *docker.Client
	manager *watcher.Manager
}

func newProcessor(set component.TelemetrySettings, cfg *Config) *dockerAttributesProcessor {
	return &dockerAttributesProcessor{
		logger: set.Logger,
		cfg:    cfg,
	}
}

func (p *dockerAttributesProcessor) Start(ctx context.Context, _ component.Host) error {
	opts := []docker.Opt{
		docker.WithHost(p.cfg.Endpoint),
		docker.WithUserAgent(processorUserAgent),
	}
	if v := p.cfg.DockerAPIVersion; v != "" {
		opts = append(opts, docker.WithVersion(v))
	} else {
		opts = append(opts, docker.WithAPIVersionNegotiation())
	}

	sdk, err := docker.NewClientWithOpts(opts...)
	if err != nil {
		return err
	}
	p.sdk = sdk

	mgr, err := watcher.NewManager(sdk, watcher.Config{
		ExcludedImages: p.cfg.ExcludedImages,
		Labels:         p.cfg.Extract.Labels,
		EnvVars:        p.cfg.Extract.EnvVars,
		Grace:          p.cfg.DeleteGracePeriod,
		Timeout:        p.cfg.Timeout,
	}, p.logger)
	if err != nil {
		_ = sdk.Close()
		p.sdk = nil
		return err
	}
	p.manager = mgr
	if err := p.manager.Start(ctx); err != nil {
		_ = sdk.Close()
		p.sdk = nil
		p.manager = nil
		return err
	}
	return nil
}

func (p *dockerAttributesProcessor) Shutdown(ctx context.Context) error {
	if p.sdk != nil {
		defer func() { _ = p.sdk.Close() }()
	}
	if p.manager == nil {
		return nil
	}
	return p.manager.Shutdown(ctx)
}

// normalizeID strips a leading "docker://" prefix and validates that the
// remaining string is exactly 64 lowercase hex characters (a full container
// ID). Short IDs (12 chars) and malformed values return "", false — a clean
// no-op that avoids a pointless inspect.
func normalizeID(raw string) (string, bool) {
	id := strings.TrimPrefix(raw, "docker://")
	if len(id) != 64 {
		return "", false
	}
	_, err := hex.DecodeString(id)
	if err != nil {
		return "", false
	}
	return id, true
}

func (p *dockerAttributesProcessor) processResource(ctx context.Context, attrs pcommon.Map) {
	v, ok := attrs.Get("container.id")
	if !ok {
		return
	}
	id, ok := normalizeID(v.Str())
	if !ok {
		return
	}
	p.manager.Enrich(ctx, id, attrs)
}

func (p *dockerAttributesProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()
	for i := range rss.Len() {
		p.processResource(ctx, rss.At(i).Resource().Attributes())
	}
	return td, nil
}

func (p *dockerAttributesProcessor) processMetrics(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rms := md.ResourceMetrics()
	for i := range rms.Len() {
		p.processResource(ctx, rms.At(i).Resource().Attributes())
	}
	return md, nil
}

func (p *dockerAttributesProcessor) processLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := range rls.Len() {
		p.processResource(ctx, rls.At(i).Resource().Attributes())
	}
	return ld, nil
}

func (p *dockerAttributesProcessor) processProfiles(ctx context.Context, pd pprofile.Profiles) (pprofile.Profiles, error) {
	rps := pd.ResourceProfiles()
	for i := range rps.Len() {
		p.processResource(ctx, rps.At(i).Resource().Attributes())
	}
	return pd, nil
}
