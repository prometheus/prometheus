package azuresf

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
)

const (
	azureSFLabel            = model.MetaLabelPrefix + "azure_sf_"
	azureSFLabelApplication = azureSFLabel + "application"
	azureSFLabelService     = azureSFLabel + "service"
	azureSFLabelPartition   = azureSFLabel + "partition"
	azureSFLabelNode        = azureSFLabel + "node"
	azureSFLabelEndpoint    = azureSFLabel + "endpoint_"

	refreshTimeout = time.Second * 15
)

var (
	azureSFSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_azure_sf_refresh_failures_total",
			Help: "Number of Azure Service Fabric SD refresh failures.",
		})
	azureSFSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_azure_sf_refresh_duration_seconds",
			Help: "The duration of Azure Service Fabric SD refresh in seconds.",
		})
)

func init() {
	prometheus.MustRegister(azureSFSDRefreshFailuresCount)
	prometheus.MustRegister(azureSFSDRefreshDuration)
}

// Discovery periodically performs Azure Service Fabric SD requests.
// It implements the TargetProvider interface.
type Discovery struct {
	cfg      *config.AzureServiceFabricSDConfig
	interval time.Duration
	sfClient *sfClient
}

func NewDiscovery(cfg *config.AzureServiceFabricSDConfig) (*Discovery, error) {
	var sfc *sfClient

	if cfg.ClientCertAndKeyFile == "" {
		sfc = &sfClient{
			origin: fmt.Sprintf("http://%s", cfg.ClusterHost),
			client: http.DefaultClient,
		}
	} else {
		fileNames := strings.Split(cfg.ClientCertAndKeyFile, ",")
		if len(fileNames) != 2 {
			return nil, errors.New("expecting 'certFile,keyFile', comma-separated")
		}

		cert, err := tls.LoadX509KeyPair(fileNames[0], fileNames[1])
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{cert},
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: true, // cfg.TlsSkipVerify,
		}
		tlsConfig.BuildNameToCertificate()

		sfc = &sfClient{
			origin: fmt.Sprintf("https://%s", cfg.ClusterHost),
			client: &http.Client{
				Transport: &http.Transport{TLSClientConfig: tlsConfig},
			},
		}
	}

	return &Discovery{
		cfg:      cfg,
		interval: time.Duration(cfg.RefreshInterval),
		sfClient: sfc,
	}, nil
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	select {
	case <-ctx.Done():
		// context is already closed, do not even perform one discovery
		return
	default:
	}

	for {
		tg, err := d.refresh()
		if err != nil {
			log.Errorf("unable to refresh during Azure Sevice Fabric discovery: %s", err)
		} else {
			select {
			case <-ctx.Done():
			case ch <- []*config.TargetGroup{tg}:
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

	}
}

func (d *Discovery) refresh() (tg *config.TargetGroup, err error) {
	defer func(start time.Time) {
		azureSFSDRefreshDuration.Observe(time.Since(start).Seconds())
		if err != nil {
			azureSFSDRefreshFailuresCount.Inc()
		}
	}(time.Now())

	// ctx with cancel after a given timeout
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel() // stops the timer

	applicationEntries, err := getApplicationEntries(ctx, d.sfClient)
	if err != nil {
		return nil, err
	}

	targets := d.makeTargets(applicationEntries)

	log.Debugf("Azure Service Fabric discovery completed.")

	return &config.TargetGroup{
		Targets: targets,
	}, nil
}

type applicationEntry struct {
	application    string
	serviceEntries []serviceEntry
	err            error
}

func getApplicationEntries(ctx context.Context, client *sfClient) ([]applicationEntry, error) {
	applications, err := client.getApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get applications: %s", err)
	}

	applicationEntries := make([]applicationEntry, len(applications))
	wg := sync.WaitGroup{}
	wg.Add(len(applications))

	for i, application := range applications {
		go func(i int, application string) {
			serviceEntries, err := getServiceEntries(ctx, client, application)
			applicationEntries[i] = applicationEntry{
				err:            err,
				application:    application,
				serviceEntries: serviceEntries,
			}
			wg.Done()
		}(i, application)
	}

	wg.Wait()

	// check for errors
	for _, entry := range applicationEntries {
		if entry.err != nil {
			return nil, entry.err
		}
		applicationEntries = append(applicationEntries, entry)
	}

	return applicationEntries, nil
}

type serviceEntry struct {
	service           string
	partitionsEntries []partitionEntry
	err               error
}

func getServiceEntries(ctx context.Context, client *sfClient, application string) ([]serviceEntry, error) {
	services, err := client.getServices(ctx, application)
	if err != nil {
		return nil, fmt.Errorf("unable to get services: %s", err)
	}

	serviceEntries := make([]serviceEntry, len(services))
	wg := sync.WaitGroup{}
	wg.Add(len(services))

	for i, service := range services {
		go func(i int, service string) {
			partitionEntries, err := getPartitionsEntries(ctx, client, application, service)
			serviceEntries[i] = serviceEntry{
				err:               err,
				service:           service,
				partitionsEntries: partitionEntries,
			}
			wg.Done()
		}(i, service)
	}

	wg.Wait()

	// check for errors
	for _, entry := range serviceEntries {
		if entry.err != nil {
			return nil, entry.err
		}
	}

	return serviceEntries, nil
}

type partitionEntry struct {
	partition     string
	nodeEndpoints []nodeAndEndpoints
	err           error
}

func getPartitionsEntries(ctx context.Context, client *sfClient, application, service string) ([]partitionEntry, error) {
	partitions, err := client.getPartitions(ctx, application, service)
	if err != nil {
		return nil, fmt.Errorf("unable to get partitions: %s", err)
	}

	partitionEntries := make([]partitionEntry, len(partitions))
	wg := sync.WaitGroup{}
	wg.Add(len(partitions))

	for i, partition := range partitions {
		go func(i int, partition string) {
			nodeEndpoints, err := client.getReplicaEndpoints(ctx, application, service, partition)
			partitionEntries[i] = partitionEntry{
				err:           err,
				partition:     partition,
				nodeEndpoints: nodeEndpoints,
			}
			wg.Done()
		}(i, partition)
	}

	wg.Wait()

	// check for errors
	for _, entry := range partitionEntries {
		if entry.err != nil {
			return nil, entry.err
		}
		partitionEntries = append(partitionEntries, entry)
	}

	return partitionEntries, nil
}

func (d *Discovery) makeTargets(applicationEntries []applicationEntry) []model.LabelSet {
	// TODO - any clever way to pre-calculate the size of this slice?
	var targets []model.LabelSet

	for _, applicationEntry := range applicationEntries {
		for _, serviceEntry := range applicationEntry.serviceEntries {
			for _, partitionEntry := range serviceEntry.partitionsEntries {
				for _, nodeEndpoints := range partitionEntry.nodeEndpoints {
					for endpointName, endpointValue := range nodeEndpoints.endpoints {
						if _, ok := d.cfg.Endpoints[endpointName]; !ok {
							// endpoint is not to be scrapped
							continue
						}

						targets = append(targets, model.LabelSet{
							azureSFLabelApplication: model.LabelValue(applicationEntry.application),
							azureSFLabelService:     model.LabelValue(serviceEntry.service),
							azureSFLabelPartition:   model.LabelValue(partitionEntry.partition),
							azureSFLabelNode:        model.LabelValue(nodeEndpoints.node),

							// the default scrape target, __address__
							model.AddressLabel: model.LabelValue(endpointValue),

							// captures de endpoint name, so relabel_configs may filter if appropriate
							azureSFLabelEndpoint + model.LabelName(endpointName): model.LabelValue(endpointValue),
						})
					}
				}
			}
		}
	}

	return targets
}
