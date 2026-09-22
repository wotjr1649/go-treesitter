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
	RuntimeVersion   string        `json:"runtime_version"`
	RuntimeCommit    string        `json:"runtime_commit"`
	TransportModule  string        `json:"transport_module"`
	TransportVersion string        `json:"transport_version"`
	TransportCommit  string        `json:"transport_commit"`
	Generator        string        `json:"generator"`
	ABIPolicy        string        `json:"abi_policy"`
	BuildFlags       []string      `json:"build_flags"`
	TypeScriptPatch  *GrammarPatch `json:"typescript_patch,omitempty"`
}

type GrammarPatch struct {
	Commit       string `json:"commit"`
	SHA256       string `json:"sha256"`
	InputsSHA256 string `json:"inputs_sha256"`
}

type Identities struct {
	Baseline Baseline           `json:"baseline"`
	Runtime  Runtime            `json:"runtime"`
	Grammars map[string]Grammar `json:"grammars"`
	Oracle   Oracle             `json:"oracle"`
}

// Runtime identifies the relocated, patched sources compiled into this module.
// Baseline separately identifies their unmodified upstream origin.
type Runtime struct {
	Module         string `json:"module"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

func Read() (Identities, error) {
	var value Identities
	err := json.Unmarshal(encoded, &value)
	return value, err
}
