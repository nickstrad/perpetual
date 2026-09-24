package registration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Both identifier domains use the same opaque, lowercase UUID syntax. The
// version bits are intentionally unrestricted.
func validateID(id string) error {
	if len(id) != 36 {
		return fmt.Errorf("identifier must have 36 ASCII bytes")
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return fmt.Errorf("identifier separator at byte %d", i)
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("identifier hex digit at byte %d", i)
		}
	}
	return nil
}

func ValidateRequestID(id string) error { return validateID(id) }
func ValidateMachineID(id string) error { return validateID(id) }

func nameEdge(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') }

func ValidateParameters(p Parameters) error {
	if len(p.Name) < 1 || len(p.Name) > 63 {
		return fmt.Errorf("name must have 1–63 ASCII bytes")
	}
	if !nameEdge(p.Name[0]) || !nameEdge(p.Name[len(p.Name)-1]) {
		return fmt.Errorf("name must start and end with lowercase letter or digit")
	}
	for i := 1; i < len(p.Name)-1; i++ {
		if !nameEdge(p.Name[i]) && p.Name[i] != '-' {
			return fmt.Errorf("invalid name byte at %d", i)
		}
	}
	if len(p.Image) < 1 || len(p.Image) > 128 {
		return fmt.Errorf("image must have 1–128 ASCII bytes")
	}
	for i := 0; i < len(p.Image); i++ {
		c := p.Image[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return fmt.Errorf("invalid image byte at %d", i)
		}
	}
	if p.VCPUs < 1 || p.VCPUs > 64 {
		return fmt.Errorf("vcpus must be 1–64")
	}
	if p.MemoryMiB < 128 || p.MemoryMiB > 1048576 {
		return fmt.Errorf("memory_mib must be 128–1048576")
	}
	if p.DiskMiB < 1024 || p.DiskMiB > 16777216 {
		return fmt.Errorf("disk_mib must be 1024–16777216")
	}
	return nil
}

// CanonicalParameters fixes both the field order and the fingerprint schema.
// Adding a behavior-affecting field requires an explicit version decision.
type CanonicalParameters struct {
	Version   uint32 `json:"version"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	VCPUs     uint32 `json:"vcpus"`
	MemoryMiB uint64 `json:"memory_mib"`
	DiskMiB   uint64 `json:"disk_mib"`
}

func Canonicalize(p Parameters) ([]byte, error) {
	if err := ValidateParameters(p); err != nil {
		return nil, err
	}
	return json.Marshal(CanonicalParameters{Version: 1, Name: p.Name, Image: p.Image, VCPUs: p.VCPUs, MemoryMiB: p.MemoryMiB, DiskMiB: p.DiskMiB})
}

func Fingerprint(p Parameters) ([32]byte, error) {
	encoded, err := Canonicalize(p)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func NewRequest(id RequestID, candidate MachineID, parameters Parameters) (Request, error) {
	if err := ValidateRequestID(string(id)); err != nil {
		return Request{}, fmt.Errorf("request ID: %w", err)
	}
	if err := ValidateMachineID(string(candidate)); err != nil {
		return Request{}, fmt.Errorf("candidate machine ID: %w", err)
	}
	digest, err := Fingerprint(parameters)
	if err != nil {
		return Request{}, fmt.Errorf("parameters: %w", err)
	}
	return Request{ID: id, CandidateID: candidate, Parameters: parameters, Fingerprint: digest}, nil
}
