// Package router provides job complexity classification and GCP service routing.
//
// EvaluateJobComplexity inspects an incoming SubmitJobRequest and returns the
// appropriate ComplexityLevel together with the GCP service that should execute
// the workload:
//
//   - SIMPLE  → Cloud Tasks   (no machine type, tiny CPU/memory, ≤ 10 min)
//   - MEDIUM  → Cloud Run Jobs (no machine type, moderate resources, ≤ 1 hour)
//   - COMPLEX → Cloud Batch   (specific machine type, heavy resources, or long duration)
package router

import (
	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
)

// ComplexityLevel represents the tier of a submitted job.
type ComplexityLevel int

const (
	ComplexityUnspecified ComplexityLevel = iota
	// ComplexitySimple: no machine-type constraint, ≤ 500 mCPU, ≤ 512 MiB, ≤ 600 s.
	ComplexitySimple
	// ComplexityMedium: no machine-type constraint but more resources or up to 3 600 s.
	ComplexityMedium
	// ComplexityComplex: explicit machine type, heavy CPU/memory, or very long duration.
	ComplexityComplex
)

// String returns a human-readable label for the complexity level.
func (c ComplexityLevel) String() string {
	switch c {
	case ComplexitySimple:
		return "SIMPLE"
	case ComplexityMedium:
		return "MEDIUM"
	case ComplexityComplex:
		return "COMPLEX"
	default:
		return "UNSPECIFIED"
	}
}

// AssignedService represents the GCP execution service that will run the job.
type AssignedService int

const (
	AssignedServiceUnspecified AssignedService = iota
	// AssignedServiceCloudTasks routes the job to GCP Cloud Tasks.
	AssignedServiceCloudTasks
	// AssignedServiceCloudRunJob routes the job to GCP Cloud Run Jobs.
	AssignedServiceCloudRunJob
	// AssignedServiceCloudBatch routes the job to GCP Cloud Batch.
	AssignedServiceCloudBatch
)

// String returns a human-readable label for the assigned service.
func (a AssignedService) String() string {
	switch a {
	case AssignedServiceCloudTasks:
		return "CLOUD_TASKS"
	case AssignedServiceCloudRunJob:
		return "CLOUD_RUN_JOB"
	case AssignedServiceCloudBatch:
		return "CLOUD_BATCH"
	default:
		return "UNSPECIFIED"
	}
}

// RoutingDecision is the output of EvaluateJobComplexity.
type RoutingDecision struct {
	Complexity      ComplexityLevel
	AssignedService AssignedService
	// Reason is a short human-readable explanation of why this tier was chosen.
	Reason string
}

// Thresholds that define tier boundaries.
//
// These constants are exported so that callers (e.g. tests, dashboards) can
// reference and override them without magic numbers.
const (
	// SimpleCPUMillisMax is the maximum CPU (in milli-cores) for a SIMPLE job.
	SimpleCPUMillisMax int64 = 500
	// SimpleMemoryMiBMax is the maximum memory (in MiB) for a SIMPLE job.
	SimpleMemoryMiBMax int64 = 512
	// SimpleDurationSecMax is the maximum duration (in seconds) for a SIMPLE job (10 min).
	SimpleDurationSecMax int64 = 600

	// MediumCPUMillisMax is the maximum CPU (in milli-cores) for a MEDIUM job.
	MediumCPUMillisMax int64 = 4000
	// MediumMemoryMiBMax is the maximum memory (in MiB) for a MEDIUM job.
	MediumMemoryMiBMax int64 = 8192
	// MediumDurationSecMax is the maximum duration (in seconds) for a MEDIUM job (1 hour).
	MediumDurationSecMax int64 = 3600
)

// EvaluateJobComplexity inspects req and returns the routing decision.
//
// Decision logic (strictest check first):
//  1. If machine_type is set → COMPLEX / Cloud Batch.
//  2. If cpu_millis > MediumCPUMillisMax, memory_mib > MediumMemoryMiBMax,
//     or max_run_duration_seconds > MediumDurationSecMax → COMPLEX / Cloud Batch.
//  3. If cpu_millis > SimpleCPUMillisMax, memory_mib > SimpleMemoryMiBMax,
//     or max_run_duration_seconds > SimpleDurationSecMax → MEDIUM / Cloud Run Jobs.
//  4. Otherwise → SIMPLE / Cloud Tasks.
//
// Zero-value resource fields are treated as "not specified" and do not push
// the job into a higher tier on their own.
func EvaluateJobComplexity(req *jennahv1.SubmitJobRequest) RoutingDecision {
	// Extract resource values; fall back to zero when no override is provided.
	var cpuMillis, memoryMiB, durationSec int64
	if ro := req.GetResourceOverride(); ro != nil {
		cpuMillis = ro.GetCpuMillis()
		memoryMiB = ro.GetMemoryMib()
		durationSec = ro.GetMaxRunDurationSeconds()
	}
	machineType := req.GetMachineType()

	// --- Rule 1: explicit machine type → always COMPLEX ---
	if machineType != "" {
		return RoutingDecision{
			Complexity:      ComplexityComplex,
			AssignedService: AssignedServiceCloudBatch,
			Reason:          "explicit machine_type requested: " + machineType,
		}
	}

	// --- Rule 2: heavy resources → COMPLEX ---
	if exceedsThreshold(cpuMillis, MediumCPUMillisMax) {
		return RoutingDecision{
			Complexity:      ComplexityComplex,
			AssignedService: AssignedServiceCloudBatch,
			Reason:          "cpu_millis exceeds medium threshold",
		}
	}
	if exceedsThreshold(memoryMiB, MediumMemoryMiBMax) {
		return RoutingDecision{
			Complexity:      ComplexityComplex,
			AssignedService: AssignedServiceCloudBatch,
			Reason:          "memory_mib exceeds medium threshold",
		}
	}
	if exceedsThreshold(durationSec, MediumDurationSecMax) {
		return RoutingDecision{
			Complexity:      ComplexityComplex,
			AssignedService: AssignedServiceCloudBatch,
			Reason:          "max_run_duration_seconds exceeds medium threshold",
		}
	}

	// --- Rule 3: moderate resources → MEDIUM ---
	if exceedsThreshold(cpuMillis, SimpleCPUMillisMax) {
		return RoutingDecision{
			Complexity:      ComplexityMedium,
			AssignedService: AssignedServiceCloudRunJob,
			Reason:          "cpu_millis exceeds simple threshold",
		}
	}
	if exceedsThreshold(memoryMiB, SimpleMemoryMiBMax) {
		return RoutingDecision{
			Complexity:      ComplexityMedium,
			AssignedService: AssignedServiceCloudRunJob,
			Reason:          "memory_mib exceeds simple threshold",
		}
	}
	if exceedsThreshold(durationSec, SimpleDurationSecMax) {
		return RoutingDecision{
			Complexity:      ComplexityMedium,
			AssignedService: AssignedServiceCloudRunJob,
			Reason:          "max_run_duration_seconds exceeds simple threshold",
		}
	}

	// --- Rule 4: everything else → SIMPLE ---
	return RoutingDecision{
		Complexity:      ComplexitySimple,
		AssignedService: AssignedServiceCloudTasks,
		Reason:          "no machine type, resources within simple thresholds",
	}
}

// exceedsThreshold returns true only when value is both non-zero and greater
// than max. A zero value means "not specified" and is not penalised.
func exceedsThreshold(value, max int64) bool {
	return value > 0 && value > max
}
