package navigator

import (
	"strings"
	"testing"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/internal/router"
)

// ─── Navigate() ─────────────────────────────────────────────────────────────

func TestNavigate_SimpleJob(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{
		ImageUri: "gcr.io/google-samples/hello-app:1.0",
		EnvVars:  map[string]string{"APP_NAME": "hello-world"},
	}
	plan, err := Navigate(req, "aaaaaaaa-0000-0000-0000-000000000001", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	if plan.Complexity != router.ComplexitySimple {
		t.Errorf("complexity: got %s, want SIMPLE", plan.Complexity)
	}
	if plan.AssignedService != router.AssignedServiceCloudTasks {
		t.Errorf("service: got %s, want CLOUD_TASKS", plan.AssignedService)
	}
	if plan.Config.ImageURI != req.ImageUri {
		t.Errorf("ImageURI: got %q, want %q", plan.Config.ImageURI, req.ImageUri)
	}
	if plan.Config.EnvVars["APP_NAME"] != "hello-world" {
		t.Errorf("EnvVars not propagated correctly")
	}
	if plan.Config.BootDiskSizeGb != defaultBootDiskGB {
		t.Errorf("BootDiskSizeGb: got %d, want default %d", plan.Config.BootDiskSizeGb, defaultBootDiskGB)
	}
	if plan.Summary == "" {
		t.Error("Summary must not be empty")
	}
	if plan.ClassifyReason == "" {
		t.Error("ClassifyReason must not be empty")
	}
}

func TestNavigate_MediumJob(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{
		ImageUri:        "gcr.io/my-project/ml-app:v2",
		ResourceProfile: "medium",
		Commands:        []string{"python", "train.py"},
		ResourceOverride: &jennahv1.ResourceOverride{
			CpuMillis: 2000,
		},
	}
	plan, err := Navigate(req, "bbbbbbbb-0000-0000-0000-000000000002", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	if plan.Complexity != router.ComplexityMedium {
		t.Errorf("complexity: got %s, want MEDIUM", plan.Complexity)
	}
	if plan.AssignedService != router.AssignedServiceCloudRunJob {
		t.Errorf("service: got %s, want CLOUD_RUN_JOB", plan.AssignedService)
	}
	if plan.Config.Resources == nil {
		t.Fatal("Resources must not be nil")
	}
	// Override of 2000 mCPU should be applied.
	if plan.Config.Resources.CPUMillis != 2000 {
		t.Errorf("CPUMillis: got %d, want 2000", plan.Config.Resources.CPUMillis)
	}
	if len(plan.Config.Commands) != 2 || plan.Config.Commands[0] != "python" {
		t.Errorf("Commands not propagated: %v", plan.Config.Commands)
	}
}

func TestNavigate_ComplexJob_MachineType(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{
		ImageUri:       "gcr.io/my-project/heavy:latest",
		MachineType:    "n1-standard-16",
		BootDiskSizeGb: 100,
		UseSpotVms:     true,
		ServiceAccount: "ml-sa@my-project.iam.gserviceaccount.com",
	}
	plan, err := Navigate(req, "cccccccc-0000-0000-0000-000000000003", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	if plan.Complexity != router.ComplexityComplex {
		t.Errorf("complexity: got %s, want COMPLEX", plan.Complexity)
	}
	if plan.AssignedService != router.AssignedServiceCloudBatch {
		t.Errorf("service: got %s, want CLOUD_BATCH", plan.AssignedService)
	}
	if plan.Config.MachineType != "n1-standard-16" {
		t.Errorf("MachineType: got %q", plan.Config.MachineType)
	}
	if plan.Config.BootDiskSizeGb != 100 {
		t.Errorf("BootDiskSizeGb: got %d, want 100", plan.Config.BootDiskSizeGb)
	}
	if !plan.Config.UseSpotVMs {
		t.Error("UseSpotVMs should be true")
	}
	if plan.Config.ServiceAccount != "ml-sa@my-project.iam.gserviceaccount.com" {
		t.Errorf("ServiceAccount: got %q", plan.Config.ServiceAccount)
	}
}

func TestNavigate_ComplexJob_HeavyResources(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{
		ImageUri: "gcr.io/my-project/bigdata:latest",
		ResourceOverride: &jennahv1.ResourceOverride{
			CpuMillis:             8000,
			MemoryMib:             16384,
			MaxRunDurationSeconds: 7200,
		},
	}
	plan, err := Navigate(req, "dddddddd-0000-0000-0000-000000000004", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	if plan.Complexity != router.ComplexityComplex {
		t.Errorf("complexity: got %s, want COMPLEX", plan.Complexity)
	}
	if plan.AssignedService != router.AssignedServiceCloudBatch {
		t.Errorf("service: got %s, want CLOUD_BATCH", plan.AssignedService)
	}
}

func TestNavigate_NilRequest(t *testing.T) {
	_, err := Navigate(nil, "some-id", nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestNavigate_EmptyJobID(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{ImageUri: "alpine:latest"}
	_, err := Navigate(req, "", nil)
	if err == nil {
		t.Error("expected error for empty jobID")
	}
}

func TestNavigate_InvalidBootDisk(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{
		ImageUri:       "alpine:latest",
		BootDiskSizeGb: 5, // below 10 GB minimum
	}
	_, err := Navigate(req, "eeeeeeee-0000-0000-0000-000000000005", nil)
	if err == nil {
		t.Error("expected error for boot_disk_size_gb < 10")
	}
}

// ─── generateProviderJobID() ─────────────────────────────────────────────────

func TestGenerateProviderJobID_WithName(t *testing.T) {
	id := generateProviderJobID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "my pipeline")
	if id != "jennah-my-pipeline" {
		t.Errorf("got %q", id)
	}
}

func TestGenerateProviderJobID_WithoutName(t *testing.T) {
	id := generateProviderJobID("abcdef12-0000-0000-0000-000000000000", "")
	if id != "jennah-abcdef12" {
		t.Errorf("got %q, want jennah-abcdef12", id)
	}
}

func TestGenerateProviderJobID_MaxLength(t *testing.T) {
	longName := strings.Repeat("a", 100)
	id := generateProviderJobID("xxxx", longName)
	if len(id) > 63 {
		t.Errorf("id too long: %d chars", len(id))
	}
}

func TestGenerateProviderJobID_SpecialChars(t *testing.T) {
	id := generateProviderJobID("xxxx", "My_Job 2026!")
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			t.Errorf("invalid char %q in id %q", c, id)
		}
	}
}

// ─── resolveBuiltinProfile() ─────────────────────────────────────────────────

func TestResolveBuiltinProfile_KnownPresets(t *testing.T) {
	cases := map[string][3]int64{
		"small":  {2000, 2048, 1800},
		"medium": {4000, 4096, 3600},
		"large":  {8000, 8192, 7200},
		"xlarge": {16000, 16384, 14400},
	}
	for name, want := range cases {
		r := resolveBuiltinProfile(name, nil)
		if r.CPUMillis != want[0] || r.MemoryMiB != want[1] || r.MaxRunDurationSeconds != want[2] {
			t.Errorf("[%s] got cpu=%d mem=%d dur=%d", name, r.CPUMillis, r.MemoryMiB, r.MaxRunDurationSeconds)
		}
	}
}

func TestResolveBuiltinProfile_UnknownFallsToMedium(t *testing.T) {
	r := resolveBuiltinProfile("nonexistent", nil)
	if r.CPUMillis != 4000 {
		t.Errorf("expected medium fallback, got cpu=%d", r.CPUMillis)
	}
}

func TestResolveBuiltinProfile_OverrideApplied(t *testing.T) {
	override := &jennahv1.ResourceOverride{
		CpuMillis: 1000,
		MemoryMib: 1024,
	}
	r := resolveBuiltinProfile("large", override)
	if r.CPUMillis != 1000 {
		t.Errorf("CPUMillis override not applied: got %d", r.CPUMillis)
	}
	if r.MemoryMiB != 1024 {
		t.Errorf("MemoryMiB override not applied: got %d", r.MemoryMiB)
	}
	// Duration not overridden — should keep large preset value.
	if r.MaxRunDurationSeconds != 7200 {
		t.Errorf("Duration should keep preset 7200, got %d", r.MaxRunDurationSeconds)
	}
}

// ─── TaskGroup defaults ───────────────────────────────────────────────────────

func TestNavigate_TaskGroupDefaults(t *testing.T) {
	req := &jennahv1.SubmitJobRequest{ImageUri: "alpine:latest"}
	plan, err := Navigate(req, "ffffffff-0000-0000-0000-000000000006", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	tg := plan.Config.TaskGroup
	if tg == nil {
		t.Fatal("TaskGroup must not be nil")
	}
	if tg.TaskCount != 1 {
		t.Errorf("TaskCount: got %d, want 1", tg.TaskCount)
	}
	if tg.SchedulingPolicy != "AS_SOON_AS_POSSIBLE" {
		t.Errorf("SchedulingPolicy: got %q", tg.SchedulingPolicy)
	}
}

// ─── EnvVars isolation ────────────────────────────────────────────────────────

func TestNavigate_EnvVarsAreCopied(t *testing.T) {
	originalEnv := map[string]string{"KEY": "val"}
	req := &jennahv1.SubmitJobRequest{
		ImageUri: "alpine:latest",
		EnvVars:  originalEnv,
	}
	plan, err := Navigate(req, "gggggggg-0000-0000-0000-000000000007", nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	// Mutating the plan config should not affect the original.
	plan.Config.EnvVars["KEY"] = "mutated"
	if originalEnv["KEY"] != "val" {
		t.Error("EnvVars map was not copied; mutation affected original request")
	}
}

// ─── RequestID for idempotency ────────────────────────────────────────────────

func TestNavigate_RequestIDIsRawUUID(t *testing.T) {
	uuid := "12345678-abcd-ef00-1234-abcdef012345"
	req := &jennahv1.SubmitJobRequest{ImageUri: "alpine:latest"}
	plan, err := Navigate(req, uuid, nil)
	if err != nil {
		t.Fatalf("Navigate() error: %v", err)
	}
	if plan.Config.RequestID != uuid {
		t.Errorf("RequestID: got %q, want %q", plan.Config.RequestID, uuid)
	}
}
