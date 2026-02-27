package router

import (
	"testing"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
)

// helper builds a SubmitJobRequest with the supplied values.
func makeReq(machineType string, cpuMillis, memoryMiB, durationSec int64) *jennahv1.SubmitJobRequest {
	return &jennahv1.SubmitJobRequest{
		ImageUri:    "gcr.io/project/image:latest",
		MachineType: machineType,
		ResourceOverride: &jennahv1.ResourceOverride{
			CpuMillis:             cpuMillis,
			MemoryMib:             memoryMiB,
			MaxRunDurationSeconds: durationSec,
		},
	}
}

// ---------------------------------------------------------------------------
// SIMPLE tier
// ---------------------------------------------------------------------------

func TestSimple_NoResources(t *testing.T) {
	// A bare job with no resource overrides and no machine type.
	req := &jennahv1.SubmitJobRequest{ImageUri: "gcr.io/project/echo:latest"}
	got := EvaluateJobComplexity(req)
	assertTier(t, "no-resource job", got, ComplexitySimple, AssignedServiceCloudTasks)
}

func TestSimple_LowResources(t *testing.T) {
	// cpu=250m, mem=256MiB, duration=300s — all within SIMPLE bounds.
	req := makeReq("", 250, 256, 300)
	got := EvaluateJobComplexity(req)
	assertTier(t, "low-resource SIMPLE job", got, ComplexitySimple, AssignedServiceCloudTasks)
}

func TestSimple_AtThreshold(t *testing.T) {
	// Exactly on the threshold values should still be SIMPLE (not exceeding them).
	req := makeReq("", SimpleCPUMillisMax, SimpleMemoryMiBMax, SimpleDurationSecMax)
	got := EvaluateJobComplexity(req)
	assertTier(t, "at-threshold SIMPLE job", got, ComplexitySimple, AssignedServiceCloudTasks)
}

func TestSimple_NilResourceOverride(t *testing.T) {
	// Explicit nil ResourceOverride must be treated as zero — stays SIMPLE.
	req := &jennahv1.SubmitJobRequest{
		ImageUri:         "gcr.io/project/hello:latest",
		ResourceOverride: nil,
	}
	got := EvaluateJobComplexity(req)
	assertTier(t, "nil resource-override SIMPLE job", got, ComplexitySimple, AssignedServiceCloudTasks)
}

// ---------------------------------------------------------------------------
// MEDIUM tier
// ---------------------------------------------------------------------------

func TestMedium_CPUExceedsSimple(t *testing.T) {
	// cpu just above 500m but below 4000m → MEDIUM.
	req := makeReq("", 1000, 256, 300)
	got := EvaluateJobComplexity(req)
	assertTier(t, "medium CPU job", got, ComplexityMedium, AssignedServiceCloudRunJob)
}

func TestMedium_MemoryExceedsSimple(t *testing.T) {
	// memory just above 512MiB → MEDIUM.
	req := makeReq("", 250, 1024, 300)
	got := EvaluateJobComplexity(req)
	assertTier(t, "medium memory job", got, ComplexityMedium, AssignedServiceCloudRunJob)
}

func TestMedium_DurationExceedsSimple(t *testing.T) {
	// 30-minute job — above 10-min simple limit, within 1-hour medium limit.
	req := makeReq("", 250, 256, 1800)
	got := EvaluateJobComplexity(req)
	assertTier(t, "medium duration job", got, ComplexityMedium, AssignedServiceCloudRunJob)
}

func TestMedium_AtMediumThreshold(t *testing.T) {
	// Exactly at medium thresholds is still MEDIUM (not exceeding).
	req := makeReq("", MediumCPUMillisMax, MediumMemoryMiBMax, MediumDurationSecMax)
	got := EvaluateJobComplexity(req)
	assertTier(t, "at medium threshold", got, ComplexityMedium, AssignedServiceCloudRunJob)
}

// ---------------------------------------------------------------------------
// COMPLEX tier — machine type
// ---------------------------------------------------------------------------

func TestComplex_ExplicitMachineType(t *testing.T) {
	// Any non-empty machine_type immediately means COMPLEX / Cloud Batch.
	cases := []string{"e2-micro", "n1-standard-16", "a2-highgpu-1g", "custom-8-32768"}
	for _, mt := range cases {
		req := makeReq(mt, 0, 0, 0)
		got := EvaluateJobComplexity(req)
		assertTier(t, "machine_type="+mt, got, ComplexityComplex, AssignedServiceCloudBatch)
	}
}

func TestComplex_MachineTypeWithResources(t *testing.T) {
	// Even if resources are tiny, machine_type drives the decision.
	req := makeReq("e2-micro", 100, 128, 60)
	got := EvaluateJobComplexity(req)
	assertTier(t, "e2-micro with tiny resources", got, ComplexityComplex, AssignedServiceCloudBatch)
}

// ---------------------------------------------------------------------------
// COMPLEX tier — resource limits
// ---------------------------------------------------------------------------

func TestComplex_CPUExceedsMedium(t *testing.T) {
	// cpu > 4000m → COMPLEX.
	req := makeReq("", 8000, 512, 300)
	got := EvaluateJobComplexity(req)
	assertTier(t, "heavy CPU job", got, ComplexityComplex, AssignedServiceCloudBatch)
}

func TestComplex_MemoryExceedsMedium(t *testing.T) {
	// memory > 8192MiB → COMPLEX.
	req := makeReq("", 500, 16384, 300)
	got := EvaluateJobComplexity(req)
	assertTier(t, "heavy memory job", got, ComplexityComplex, AssignedServiceCloudBatch)
}

func TestComplex_DurationExceedsMedium(t *testing.T) {
	// 2-hour job → COMPLEX.
	req := makeReq("", 500, 512, 7200)
	got := EvaluateJobComplexity(req)
	assertTier(t, "long duration job", got, ComplexityComplex, AssignedServiceCloudBatch)
}

// ---------------------------------------------------------------------------
// String helpers
// ---------------------------------------------------------------------------

func TestComplexityLevelString(t *testing.T) {
	cases := map[ComplexityLevel]string{
		ComplexityUnspecified: "UNSPECIFIED",
		ComplexitySimple:      "SIMPLE",
		ComplexityMedium:      "MEDIUM",
		ComplexityComplex:     "COMPLEX",
	}
	for level, want := range cases {
		if got := level.String(); got != want {
			t.Errorf("ComplexityLevel(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func TestAssignedServiceString(t *testing.T) {
	cases := map[AssignedService]string{
		AssignedServiceUnspecified: "UNSPECIFIED",
		AssignedServiceCloudTasks:  "CLOUD_TASKS",
		AssignedServiceCloudRunJob: "CLOUD_RUN_JOB",
		AssignedServiceCloudBatch:  "CLOUD_BATCH",
	}
	for svc, want := range cases {
		if got := svc.String(); got != want {
			t.Errorf("AssignedService(%d).String() = %q, want %q", svc, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Reason field is always populated
// ---------------------------------------------------------------------------

func TestReasonIsNeverEmpty(t *testing.T) {
	reqs := []*jennahv1.SubmitJobRequest{
		{ImageUri: "gcr.io/project/echo:latest"},
		makeReq("", 1000, 256, 300),
		makeReq("", 500, 512, 7200),
		makeReq("e2-micro", 0, 0, 0),
	}
	for _, req := range reqs {
		d := EvaluateJobComplexity(req)
		if d.Reason == "" {
			t.Errorf("EvaluateJobComplexity(%v) returned empty Reason", req)
		}
	}
}

// ---------------------------------------------------------------------------
// assertion helper
// ---------------------------------------------------------------------------

func assertTier(t *testing.T, name string, got RoutingDecision, wantLevel ComplexityLevel, wantSvc AssignedService) {
	t.Helper()
	if got.Complexity != wantLevel {
		t.Errorf("[%s] complexity: got %s, want %s (reason: %s)",
			name, got.Complexity, wantLevel, got.Reason)
	}
	if got.AssignedService != wantSvc {
		t.Errorf("[%s] service: got %s, want %s (reason: %s)",
			name, got.AssignedService, wantSvc, got.Reason)
	}
}
