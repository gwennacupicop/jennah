// Package dispatcher routes job requests to the appropriate GCP service provider
// based on the complexity classification from the router.
//
// It holds references to all configured providers (Cloud Tasks, Cloud Run Jobs,
// Cloud Batch) and selects the correct one for each job submission.
package dispatcher

import (
	"context"
	"fmt"
	"log"

	batch "github.com/alphauslabs/jennah/internal/cloudexec"
	"github.com/alphauslabs/jennah/internal/router"
)

// Dispatcher routes job operations to the appropriate cloud service provider
// based on the router's AssignedService decision.
type Dispatcher struct {
	// providers maps router.AssignedService → batch.Provider.
	providers map[router.AssignedService]batch.Provider
}

// New creates a new Dispatcher with the given providers.
// At least one provider must be supplied. Missing providers for a given
// service tier will cause SubmitJob to return an error for that tier.
func New(opts ...Option) (*Dispatcher, error) {
	d := &Dispatcher{
		providers: make(map[router.AssignedService]batch.Provider),
	}
	for _, opt := range opts {
		opt(d)
	}

	if len(d.providers) == 0 {
		return nil, fmt.Errorf("dispatcher: at least one provider must be configured")
	}

	// Log registered providers.
	for svc, p := range d.providers {
		log.Printf("Dispatcher: registered %s provider (service_type=%s)", svc, p.ServiceType())
	}

	return d, nil
}

// Option configures a Dispatcher.
type Option func(*Dispatcher)

// WithCloudRunJobs registers a Cloud Run Jobs provider for SIMPLE jobs.
func WithCloudRunJobs(p batch.Provider) Option {
	return func(d *Dispatcher) {
		d.providers[router.AssignedServiceCloudRunJob] = p
	}
}

// WithCloudBatch registers a Cloud Batch provider for COMPLEX jobs.
func WithCloudBatch(p batch.Provider) Option {
	return func(d *Dispatcher) {
		d.providers[router.AssignedServiceCloudBatch] = p
	}
}

// ProviderFor returns the provider registered for the given service tier.
// Returns an error if no provider is registered for that tier.
func (d *Dispatcher) ProviderFor(svc router.AssignedService) (batch.Provider, error) {
	p, ok := d.providers[svc]
	if !ok {
		return nil, fmt.Errorf("dispatcher: no provider registered for service %s", svc)
	}
	return p, nil
}

// SubmitJob submits a job using the provider that matches assignedService.
func (d *Dispatcher) SubmitJob(ctx context.Context, assignedService router.AssignedService, config batch.JobConfig) (*batch.JobResult, error) {
	p, err := d.ProviderFor(assignedService)
	if err != nil {
		return nil, err
	}

	log.Printf("Dispatcher: routing job %s to %s", config.JobID, assignedService)
	return p.SubmitJob(ctx, config)
}

// GetJobStatus retrieves job status using the provider that matches assignedService.
func (d *Dispatcher) GetJobStatus(ctx context.Context, assignedService router.AssignedService, cloudResourcePath string) (batch.JobStatus, error) {
	p, err := d.ProviderFor(assignedService)
	if err != nil {
		return batch.JobStatusUnknown, err
	}

	return p.GetJobStatus(ctx, cloudResourcePath)
}

// CancelJob cancels a job using the provider that matches assignedService.
func (d *Dispatcher) CancelJob(ctx context.Context, assignedService router.AssignedService, cloudResourcePath string) error {
	p, err := d.ProviderFor(assignedService)
	if err != nil {
		return err
	}

	return p.CancelJob(ctx, cloudResourcePath)
}
