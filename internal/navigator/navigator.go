// Package navigator is the task load-balancer / navigator layer for Jennah.
//
// It sits between the incoming SubmitJobRequest and the batch execution layer:
//
//	SubmitJobRequest
//	    ↓
//	navigator.Navigate()          ← you are here
//	    ├─ router.EvaluateJobComplexity()  — classify SIMPLE / MEDIUM / COMPLEX
//	    ├─ buildJobConfig()                — translate all proto fields → JobConfig
//	    └─ NavigationPlan                  — complete, ready-to-execute plan
//	         ↓
//	GCP Cloud Tasks / Cloud Run Jobs / Cloud Batch
//
// The navigator is deliberately stateless (no I/O). It only transforms data so
// it is easy to unit-test and safe to call from any goroutine.
package navigator

import (
	"fmt"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
	"github.com/alphauslabs/jennah/internal/router"
)

// NavigationPlan is the full, resolved execution plan for a submitted job.
// Callers should inspect AssignedService to decide which GCP API to call,
// then pass Config directly to the appropriate provider.
type NavigationPlan struct {
	// ── Routing decision ──────────────────────────────────────────────────────

	// Complexity is the evaluated tier: SIMPLE, MEDIUM, or COMPLEX.
	Complexity router.ComplexityLevel

	// AssignedService is the GCP service that will execute this job.
	//   SIMPLE  → Cloud Tasks
	//   MEDIUM  → Cloud Run Jobs
	//   COMPLEX → Cloud Batch
	AssignedService router.AssignedService

	// ClassifyReason is a human-readable explanation of the routing decision.
	ClassifyReason string

	// ── Execution config ──────────────────────────────────────────────────────

	// Config is the fully-populated JobConfig ready to pass to the batch provider
	// or any downstream GCP API adapter.
	Config batch.JobConfig

	// ── Summary ───────────────────────────────────────────────────────────────

	// Summary is a one-line human-readable description of the plan.
	// Useful for logging and audit trails.
	Summary string
}

// Navigate is the single entry point for the navigator/load-balancer.
//
// It accepts:
//   - req   : the validated SubmitJobRequest from the gateway
//   - jobID : a pre-generated UUID (used for idempotency + DB record linking)
//   - cfg   : loaded job-config.json (resource profiles)
//
// It returns a NavigationPlan with all fields populated, or an error if the
// request cannot be mapped to a valid execution plan.
func Navigate(req *jennahv1.SubmitJobRequest, jobID string, cfg *config.JobConfigFile) (*NavigationPlan, error) {
	if req == nil {
		return nil, fmt.Errorf("navigator: request must not be nil")
	}
	if jobID == "" {
		return nil, fmt.Errorf("navigator: jobID must not be empty")
	}

	// Step 1 — Classify complexity and select target GCP service.
	decision := router.EvaluateJobComplexity(req)

	// Step 2 — Build the full JobConfig (field translation + resource resolution).
	jobCfg, err := buildJobConfig(req, jobID, cfg)
	if err != nil {
		return nil, fmt.Errorf("navigator: failed to build job config: %w", err)
	}

	// Step 3 — Assemble and return the navigation plan.
	plan := &NavigationPlan{
		Complexity:      decision.Complexity,
		AssignedService: decision.AssignedService,
		ClassifyReason:  decision.Reason,
		Config:          jobCfg,
		Summary: fmt.Sprintf(
			"job=%s tier=%s service=%s image=%s",
			jobID,
			decision.Complexity,
			decision.AssignedService,
			req.GetImageUri(),
		),
	}

	return plan, nil
}
