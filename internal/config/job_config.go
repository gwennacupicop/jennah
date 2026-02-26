package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alphauslabs/jennah/internal/batch"
)

// JobConfigFile represents the structure of the job configuration JSON file.
type JobConfigFile struct {
	DefaultResources     ResourceProfile            `json:"defaultResources"`
	ResourceProfiles     map[string]ResourceProfile `json:"resourceProfiles"`
	MachineTypeResources map[string]ResourceProfile `json:"machineTypeResources"`
}

// ResourceProfile defines resource requirements for a job.
type ResourceProfile struct {
	CPUMillis             int64 `json:"cpuMillis"`
	MemoryMiB             int64 `json:"memoryMiB"`
	MaxRunDurationSeconds int64 `json:"maxRunDurationSeconds"`
}

// LoadJobConfig loads job configuration from a JSON file.
func LoadJobConfig(filePath string) (*JobConfigFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config JobConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	return &config, nil
}

// GetResourceRequirements returns resource requirements for a profile name.
// If profileName is empty or not found, returns default resources.
func (c *JobConfigFile) GetResourceRequirements(profileName string) *batch.ResourceRequirements {
	var profile ResourceProfile

	if profileName != "" {
		if p, exists := c.ResourceProfiles[profileName]; exists {
			profile = p
		} else {
			profile = c.DefaultResources
		}
	} else {
		profile = c.DefaultResources
	}

	return &batch.ResourceRequirements{
		CPUMillis:             profile.CPUMillis,
		MemoryMiB:             profile.MemoryMiB,
		MaxRunDurationSeconds: profile.MaxRunDurationSeconds,
	}
}

// GetMachineTypeResources returns resource requirements for a machine type.
// If machineType is empty or not found, returns nil.
func (c *JobConfigFile) GetMachineTypeResources(machineType string) *batch.ResourceRequirements {
	if machineType == "" {
		return nil
	}

	if profile, exists := c.MachineTypeResources[machineType]; exists {
		return &batch.ResourceRequirements{
			CPUMillis:             profile.CPUMillis,
			MemoryMiB:             profile.MemoryMiB,
			MaxRunDurationSeconds: profile.MaxRunDurationSeconds,
		}
	}

	return nil
}

// ResourceOverride holds optional per-field overrides for compute resources.
// A zero value for any field means "use the preset value instead".
type ResourceOverride struct {
	CPUMillis             int64
	MemoryMiB             int64
	MaxRunDurationSeconds int64
}

// ResolveResources returns the effective resource requirements by merging a
// machine type, named preset, and optional per-field override.
//
// Resolution order (highest to lowest priority):
//  1. Non-zero fields in override
//  2. Machine type resources (if machineType is provided and found)
//  3. Named preset (or default if profileName is empty or unknown)
func (c *JobConfigFile) ResolveResources(machineType string, profileName string, override *ResourceOverride) *batch.ResourceRequirements {
	var base *batch.ResourceRequirements

	// Try machine type first
	if machineType != "" {
		if mtResources := c.GetMachineTypeResources(machineType); mtResources != nil {
			base = mtResources
		}
	}

	// Fall back to profile-based resources
	if base == nil {
		base = c.GetResourceRequirements(profileName)
	}

	if override == nil {
		return base
	}

	if override.CPUMillis != 0 {
		base.CPUMillis = override.CPUMillis
	}
	if override.MemoryMiB != 0 {
		base.MemoryMiB = override.MemoryMiB
	}
	if override.MaxRunDurationSeconds != 0 {
		base.MaxRunDurationSeconds = override.MaxRunDurationSeconds
	}

	return base
}
