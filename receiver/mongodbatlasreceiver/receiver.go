// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package mongodbatlasreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbatlasreceiver"

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/atlas/mongodbatlas"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/scraperhelper"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbatlasreceiver/internal"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbatlasreceiver/internal/metadata"
)

type mongodbatlasreceiver struct {
	log         *zap.Logger
	cfg         *Config
	client      *internal.MongoDBAtlasClient
	lastRun     time.Time
	mb          *metadata.MetricsBuilder
	stopperChan chan struct{}
}

type timeconstraints struct {
	start      string
	end        string
	resolution string
}

func newMongoDBAtlasReceiver(settings receiver.CreateSettings, cfg *Config) *mongodbatlasreceiver {
	client := internal.NewMongoDBAtlasClient(cfg.PublicKey, string(cfg.PrivateKey), cfg.RetrySettings, settings.Logger)
	return &mongodbatlasreceiver{
		log:         settings.Logger,
		cfg:         cfg,
		client:      client,
		mb:          metadata.NewMetricsBuilder(cfg.MetricsBuilderConfig, settings),
		stopperChan: make(chan struct{}),
	}
}

func newMongoDBAtlasScraper(recv *mongodbatlasreceiver) (scraperhelper.Scraper, error) {
	return scraperhelper.NewScraper(metadata.Type, recv.scrape, scraperhelper.WithShutdown(recv.shutdown))
}

func (s *mongodbatlasreceiver) scrape(ctx context.Context) (pmetric.Metrics, error) {
	now := time.Now()
	if err := s.poll(ctx, s.timeConstraints(now)); err != nil {
		return pmetric.Metrics{}, err
	}
	s.lastRun = now
	return s.mb.Emit(), nil
}

func (s *mongodbatlasreceiver) timeConstraints(now time.Time) timeconstraints {
	var start time.Time
	if s.lastRun.IsZero() {
		start = now.Add(s.cfg.CollectionInterval * -1)
	} else {
		start = s.lastRun
	}
	return timeconstraints{
		start.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
		s.cfg.Granularity,
	}
}

func (s *mongodbatlasreceiver) shutdown(context.Context) error {
	return s.client.Shutdown()
}

func (s *mongodbatlasreceiver) poll(ctx context.Context, time timeconstraints) error {
	orgs, err := s.client.Organizations(ctx)
	if err != nil {
		return fmt.Errorf("error retrieving organizations: %w", err)
	}
	for _, org := range orgs {
		projects, err := s.client.Projects(ctx, org.ID)
		if err != nil {
			return fmt.Errorf("error retrieving projects: %w", err)
		}
		for _, project := range projects {
			nodeClusterMap, providerMap, err := s.getNodeClusterNameMap(ctx, project.ID)
			if err != nil {
				return fmt.Errorf("error collecting clusters from project %s: %w", project.ID, err)
			}

			processes, err := s.client.Processes(ctx, project.ID)
			if err != nil {
				return fmt.Errorf("error retrieving MongoDB Atlas processes for project %s: %w", project.ID, err)
			}
			for _, process := range processes {
				clusterName := nodeClusterMap[process.UserAlias]
				providerValues := providerMap[clusterName]

				if err := s.extractProcessMetrics(ctx, time, org.Name, project, process, clusterName, providerValues); err != nil {
					return fmt.Errorf("error when polling process metrics from MongoDB Atlas for process %s: %w", process.ID, err)
				}

				if err := s.extractProcessDatabaseMetrics(ctx, time, org.Name, project, process, clusterName, providerValues); err != nil {
					return fmt.Errorf("error when polling process database metrics from MongoDB Atlas for process %s: %w", process.ID, err)
				}

				if err := s.extractProcessDiskMetrics(ctx, time, org.Name, project, process, clusterName, providerValues); err != nil {
					return fmt.Errorf("error when polling process disk metrics from MongoDB Atlas for process %s: %w", process.ID, err)
				}
			}
		}
	}
	return nil
}

type providerValues struct {
	RegionName   string
	ProviderName string
}

func (s *mongodbatlasreceiver) getNodeClusterNameMap(
	ctx context.Context,
	projectID string,
) (map[string]string, map[string]providerValues, error) {
	providerMap := make(map[string]providerValues)
	clusterMap := make(map[string]string)
	clusters, err := s.client.GetClusters(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	for _, cluster := range clusters {
		// URI in the form mongodb://host1.mongodb.net:27017,host2.mongodb.net:27017,host3.mongodb.net:27017
		nodes := strings.Split(strings.TrimPrefix(cluster.MongoURI, "mongodb://"), ",")
		for _, node := range nodes {
			// Remove the port from the node
			n, _, _ := strings.Cut(node, ":")
			clusterMap[n] = cluster.Name

		}

		providerMap[cluster.Name] = providerValues{
			RegionName:   cluster.ProviderSettings.RegionName,
			ProviderName: cluster.ProviderSettings.ProviderName,
		}
	}

	return clusterMap, providerMap, nil
}

func (s *mongodbatlasreceiver) extractProcessMetrics(
	ctx context.Context,
	time timeconstraints,
	orgName string,
	project *mongodbatlas.Project,
	process *mongodbatlas.Process,
	clusterName string,
	providerValues providerValues,
) error {
	if err := s.client.ProcessMetrics(
		ctx,
		s.mb,
		project.ID,
		process.Hostname,
		process.Port,
		time.start,
		time.end,
		time.resolution,
	); err != nil {
		return fmt.Errorf("error when polling process metrics from MongoDB Atlas: %w", err)
	}

	rb := s.mb.NewResourceBuilder()
	rb.SetMongodbAtlasOrgName(orgName)
	rb.SetMongodbAtlasProjectName(project.Name)
	rb.SetMongodbAtlasProjectID(project.ID)
	rb.SetMongodbAtlasHostName(process.Hostname)
	rb.SetMongodbAtlasUserAlias(process.UserAlias)
	rb.SetMongodbAtlasClusterName(clusterName)
	rb.SetMongodbAtlasProcessPort(strconv.Itoa(process.Port))
	rb.SetMongodbAtlasProcessTypeName(process.TypeName)
	rb.SetMongodbAtlasProcessID(process.ID)
	rb.SetMongodbAtlasRegionName(providerValues.RegionName)
	rb.SetMongodbAtlasProviderName(providerValues.ProviderName)
	s.mb.EmitForResource(metadata.WithResource(rb.Emit()))

	return nil
}

func (s *mongodbatlasreceiver) extractProcessDatabaseMetrics(
	ctx context.Context,
	time timeconstraints,
	orgName string,
	project *mongodbatlas.Project,
	process *mongodbatlas.Process,
	clusterName string,
	providerValues providerValues,
) error {
	processDatabases, err := s.client.ProcessDatabases(
		ctx,
		project.ID,
		process.Hostname,
		process.Port,
	)
	if err != nil {
		return fmt.Errorf("error retrieving process databases: %w", err)
	}

	for _, db := range processDatabases {
		if err := s.client.ProcessDatabaseMetrics(
			ctx,
			s.mb,
			project.ID,
			process.Hostname,
			process.Port,
			db.DatabaseName,
			time.start,
			time.end,
			time.resolution,
		); err != nil {
			return fmt.Errorf("error when polling database metrics from MongoDB Atlas: %w", err)
		}
		rb := s.mb.NewResourceBuilder()
		rb.SetMongodbAtlasOrgName(orgName)
		rb.SetMongodbAtlasProjectName(project.Name)
		rb.SetMongodbAtlasProjectID(project.ID)
		rb.SetMongodbAtlasHostName(process.Hostname)
		rb.SetMongodbAtlasUserAlias(process.UserAlias)
		rb.SetMongodbAtlasClusterName(clusterName)
		rb.SetMongodbAtlasProcessPort(strconv.Itoa(process.Port))
		rb.SetMongodbAtlasProcessTypeName(process.TypeName)
		rb.SetMongodbAtlasProcessID(process.ID)
		rb.SetMongodbAtlasDbName(db.DatabaseName)
		rb.SetMongodbAtlasRegionName(providerValues.RegionName)
		rb.SetMongodbAtlasProviderName(providerValues.ProviderName)
		s.mb.EmitForResource(metadata.WithResource(rb.Emit()))
	}
	return nil
}

func (s *mongodbatlasreceiver) extractProcessDiskMetrics(
	ctx context.Context,
	time timeconstraints,
	orgName string,
	project *mongodbatlas.Project,
	process *mongodbatlas.Process,
	clusterName string,
	providerValues providerValues,
) error {
	for _, disk := range s.client.ProcessDisks(ctx, project.ID, process.Hostname, process.Port) {
		if err := s.client.ProcessDiskMetrics(
			ctx,
			s.mb,
			project.ID,
			process.Hostname,
			process.Port,
			disk.PartitionName,
			time.start,
			time.end,
			time.resolution,
		); err != nil {
			return fmt.Errorf("error when polling disk metrics from MongoDB Atlas: %w", err)
		}
		rb := s.mb.NewResourceBuilder()
		rb.SetMongodbAtlasOrgName(orgName)
		rb.SetMongodbAtlasProjectName(project.Name)
		rb.SetMongodbAtlasProjectID(project.ID)
		rb.SetMongodbAtlasHostName(process.Hostname)
		rb.SetMongodbAtlasUserAlias(process.UserAlias)
		rb.SetMongodbAtlasClusterName(clusterName)
		rb.SetMongodbAtlasProcessPort(strconv.Itoa(process.Port))
		rb.SetMongodbAtlasProcessTypeName(process.TypeName)
		rb.SetMongodbAtlasProcessID(process.ID)
		rb.SetMongodbAtlasDiskPartition(disk.PartitionName)
		rb.SetMongodbAtlasRegionName(providerValues.RegionName)
		rb.SetMongodbAtlasProviderName(providerValues.ProviderName)
		s.mb.EmitForResource(metadata.WithResource(rb.Emit()))
	}
	return nil
}
