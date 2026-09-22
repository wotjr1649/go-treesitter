// Package provenance exposes the identity tuple used by validation.
package provenance

import (
	_ "embed"
	"encoding/json"
)

//go:embed identities.json
var encoded []byte

type Baseline struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
}

type Grammar struct {
	BlobSHA256 string `json:"blob_sha256"`
	Commit     string `json:"commit"`
}

type Oracle struct {
	RuntimeVersion   string   `json:"runtime_version"`
	RuntimeCommit    string   `json:"runtime_commit"`
	TransportModule  string   `json:"transport_module"`
	TransportVersion string   `json:"transport_version"`
	TransportCommit  string   `json:"transport_commit"`
	Generator        string   `json:"generator"`
	PreferredABI     int      `json:"preferred_abi"`
	FallbackABI      int      `json:"fallback_abi"`
	BuildFlags       []string `json:"build_flags"`
}

type Identities struct {
	Baseline Baseline           `json:"baseline"`
	Grammars map[string]Grammar `json:"grammars"`
	Oracle   Oracle             `json:"oracle"`
}

func Read() (Identities, error) {
	var value Identities
	err := json.Unmarshal(encoded, &value)
	return value, err
}
