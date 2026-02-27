package navigator

import (
	"fmt"
	"strings"

	jennahv1 "github.com/alphauslabs/jennah/gen/proto"
	"github.com/alphauslabs/jennah/internal/batch"
	"github.com/alphauslabs/jennah/internal/config"
)

// defaultBootDiskGB is used when the request does not specify a boot disk size.
const defaultBootDiskGB int64 = 50

// buildJobConfig translates a SubmitJobRequest into a fully-populated JobConfig.
//
// Field mapping (SubmitJobRequest → JobConfig):
//
//	image_uri            → ImageURI
//	commands             → Commands
//	env_vars             → EnvVars
//	resource_profile
//	  + resource_override → Resources  (resolved via config.ResolveResources)
//	machine_type         → MachineType
//	boot_disk_size_gb    → BootDiskSizeGb  (default 50 if 0)
//	use_spot_vms         → UseSpotVMs
//	service_account      → ServiceAccount
//	name                 → Name  (also used in generateProviderJobID)
//	jobID                → JobID (provider-compatible) + RequestID (idempotency)
func buildJobConfig(
	req *jennahv1.SubmitJobRequest,
	jobID string,
	cfg *config.JobConfigFile,
) (batch.JobConfig, error) {

	// ── Resource resolution ───────────────────────────────────────────────────
	// Merge the named preset (resource_profile) with any per-field overrides
	// (resource_override).  A nil cfg falls back to built-in defaults.
	var resources *batch.ResourceRequirements
	if cfg != nil {
		var override *config.ResourceOverride
		if ro := req.GetResourceOverride(); ro != nil {
			override = &config.ResourceOverride{
				CPUMillis:             ro.GetCpuMillis(),
				MemoryMiB:             ro.GetMemoryMib(),
				MaxRunDurationSeconds: ro.GetMaxRunDurationSeconds(),
			}
		}
		resources = cfg.ResolveResources(req.GetMachineType(), req.GetResourceProfile(), override)
	} else {
		// No config file — fall back to "medium" hard-coded defaults so that
		// the navigator is always usable in tests and minimal deployments.
		resources = resolveBuiltinProfile(req.GetResourceProfile(), req.GetResourceOverride())
	}

	// ── Validation ────────────────────────────────────────────────────────────
	if req.GetBootDiskSizeGb() > 0 && req.GetBootDiskSizeGb() < 10 {
		return batch.JobConfig{}, fmt.Errorf(
			"boot_disk_size_gb must be ≥ 10 GB (got %d)", req.GetBootDiskSizeGb(),
		)
	}

	// ── Provider-compatible job ID ────────────────────────────────────────────
	// GCP Batch job IDs: alphanumeric + hyphens, ≤ 63 chars.
	providerJobID := generateProviderJobID(jobID, req.GetName())

	// ── Boot disk ─────────────────────────────────────────────────────────────
	bootDisk := req.GetBootDiskSizeGb()
	if bootDisk == 0 {
		bootDisk = defaultBootDiskGB
	}

	// ── Env vars ──────────────────────────────────────────────────────────────
	// Clone the map so mutations downstream don't affect the original request.
	envVars := make(map[string]string, len(req.GetEnvVars()))
	for k, v := range req.GetEnvVars() {
		envVars[k] = v
	}

	// ── Task group defaults ───────────────────────────────────────────────────
	taskGroup := &batch.TaskGroupConfig{
		TaskCount:        1,
		SchedulingPolicy: "AS_SOON_AS_POSSIBLE",
	}

	return batch.JobConfig{
		// Identity
		JobID:     providerJobID,
		RequestID: jobID, // raw UUID used for GCP idempotency key
		Name:      req.GetName(),

		// Container
		ImageURI: req.GetImageUri(),
		Commands: req.GetCommands(),
		EnvVars:  envVars,

		// Resources
		Resources:      resources,
		MachineType:    req.GetMachineType(),
		BootDiskSizeGb: bootDisk,
		UseSpotVMs:     req.GetUseSpotVms(),

		// Security & networking
		ServiceAccount: req.GetServiceAccount(),

		// Task group
		TaskGroup: taskGroup,
	}, nil
}

// generateProviderJobID produces a GCP Batch-compatible job ID (≤ 63 chars,
// alphanumeric + hyphens only).
//
//   - If name is provided: "jennah-{sanitised-name}"
//   - Otherwise:           "jennah-{uuid[:8]}"
func generateProviderJobID(uuid, name string) string {
	const prefix = "jennah-"
	const maxLen = 63

	var suffix string
	if name != "" {
		// Sanitise: lowercase, replace non-alphanumeric with hyphens.
		suffix = sanitiseLabel(name)
	} else {
		// Use first 8 hex chars of the UUID (strip hyphens for compactness).
		raw := strings.ReplaceAll(uuid, "-", "")
		if len(raw) > 8 {
			raw = raw[:8]
		}
		suffix = raw
	}

	id := prefix + suffix
	if len(id) > maxLen {
		id = id[:maxLen]
	}
	return id
}

// sanitiseLabel lowercases s and replaces any character that is not
// alphanumeric with a hyphen, collapsing consecutive hyphens.
func sanitiseLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else {
			if !prevHyphen {
				b.WriteRune('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// resolveBuiltinProfile resolves resources from hard-coded presets when no
// config file is available.  Mirrors the profiles in config/job-config.json.
func resolveBuiltinProfile(profile string, ro *jennahv1.ResourceOverride) *batch.ResourceRequirements {
	presets := map[string]batch.ResourceRequirements{
		"small":  {CPUMillis: 2000, MemoryMiB: 2048, MaxRunDurationSeconds: 1800},
		"medium": {CPUMillis: 4000, MemoryMiB: 4096, MaxRunDurationSeconds: 3600},
		"large":  {CPUMillis: 8000, MemoryMiB: 8192, MaxRunDurationSeconds: 7200},
		"xlarge": {CPUMillis: 16000, MemoryMiB: 16384, MaxRunDurationSeconds: 14400},
	}

	base, ok := presets[profile]
	if !ok {
		base = presets["medium"] // default
	}

	result := base // copy
	if ro != nil {
		if ro.GetCpuMillis() != 0 {
			result.CPUMillis = ro.GetCpuMillis()
		}
		if ro.GetMemoryMib() != 0 {
			result.MemoryMiB = ro.GetMemoryMib()
		}
		if ro.GetMaxRunDurationSeconds() != 0 {
			result.MaxRunDurationSeconds = ro.GetMaxRunDurationSeconds()
		}
	}
	return &result
}
