// SPDX-License-Identifier: Apache-2.0

// Package weaverevidence verifies the complete promoted Weaver artifact
// closure before the typed compiler is allowed to consume model or diff data.
package weaverevidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	diffPath = "diff/v1.26.0-to-v1.41.0/diff.json"

	expectedWeaverRelease = "v0.25.0"
	expectedWeaverCommit  = "a177fb605e66ef8d3b333302459c805091bfedd7"

	expectedSourceTag     = "v1.26.0"
	expectedSourceCommit  = "9adff435c76ea3b8cba89babefc832d3dd3c1ab9"
	expectedSourceArchive = "upstream/registry/semantic-conventions-v1.26.0-model.tar"
	expectedSourceSHA     = "ecf8658f85d2e06281470ec1d11d27534a6090f6e689343e757ba4ab7e1d1ece"

	expectedTargetTag     = "v1.41.0"
	expectedTargetCommit  = "e018fe6f91862f5ed63c082f87697cddac596784"
	expectedTargetArchive = "upstream/registry/semantic-conventions-v1.41.0-model.tar"
	expectedTargetSHA     = "6e2a13d9ccac76327bbb79578eced858b14fddbc96facbf56aab16a5b19cee93"
)

var expectedArtifacts = []string{
	diffPath,
	"findings-check.json",
	"findings-diff.json",
	"findings-generate.json",
	"registry-model.json",
	"registry-review.md",
}

var expectedFiles = append(append([]string(nil), expectedArtifacts...), "provenance.json")

var expectedLocalInputs = []string{
	"checksums.txt",
	"input-lock.json",
	"local/policies/otelmap-canonical.rego",
	"local/templates/registry/otelmap-weave/registry-model.json.j2",
	"local/templates/registry/otelmap-weave/registry-review.md.j2",
	"local/templates/registry/otelmap-weave/weaver.yaml",
}

type platformPin struct {
	archive string
	sum     string
	binary  string
}

var platforms = map[string]platformPin{
	"darwin/arm64": {
		archive: "upstream/weaver/weaver-aarch64-apple-darwin.tar.xz",
		sum:     "32f1c4e9092f0ba4fe48b259ce3dc1d65c756db7f759fadf5338ebacdf53a3df",
		binary:  "weaver-aarch64-apple-darwin",
	},
	"linux/amd64": {
		archive: "upstream/weaver/weaver-x86_64-unknown-linux-gnu.tar.xz",
		sum:     "8cbf46fc0137311eab7fd3860cb841031bb772f8613acc5923fa9ef3879c0cd5",
		binary:  "weaver-x86_64-unknown-linux-gnu",
	},
}

// Artifact identifies one verified file relative to the Weaver evidence root.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Bundle contains the verified compiler inputs and a digest over the complete
// Weaver evidence tree, including provenance.
type Bundle struct {
	Model            []byte
	Diff             []byte
	FindingsCheck    []byte
	FindingsGenerate []byte
	FindingsDiff     []byte
	Provenance       []byte
	EvidenceSHA256   string
	Artifacts        []Artifact
}

type provenance struct {
	SchemaVersion int                `json:"schema_version"`
	Weaver        provenanceWeaver   `json:"weaver"`
	Source        provenanceRegistry `json:"source_registry"`
	Target        provenanceRegistry `json:"target_registry"`
	Inputs        []Artifact         `json:"local_inputs"`
	Commands      []command          `json:"commands"`
	Artifacts     []Artifact         `json:"artifacts"`
}

type provenanceWeaver struct {
	Release          string `json:"release"`
	Commit           string `json:"commit"`
	Platform         string `json:"platform"`
	Archive          string `json:"archive"`
	ArchiveSHA256    string `json:"archive_sha256"`
	ExecutableSHA256 string `json:"executable_sha256"`
}

type provenanceRegistry struct {
	Tag           string `json:"tag"`
	Commit        string `json:"commit"`
	Archive       string `json:"archive"`
	ArchiveSHA256 string `json:"archive_sha256"`
}

type command struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}

// Verify rejects any missing, extra, linked, malformed, identity-mismatched or
// digest-mismatched evidence before returning compiler-consumable bytes.
func Verify(fixtureRoot, evidenceRoot string) (Bundle, error) {
	if fixtureRoot == "" || evidenceRoot == "" {
		return Bundle{}, errors.New("fixture root and Weaver evidence root are required")
	}
	if err := verifyRoot(evidenceRoot); err != nil {
		return Bundle{}, err
	}
	files, err := readEvidenceFiles(evidenceRoot)
	if err != nil {
		return Bundle{}, err
	}
	var document provenance
	if err := strictDecode(files["provenance.json"], &document); err != nil {
		return Bundle{}, fmt.Errorf("decode Weaver provenance: %w", err)
	}
	if err := verifyProvenance(fixtureRoot, files, document); err != nil {
		return Bundle{}, err
	}
	for _, path := range []string{
		"findings-check.json",
		"findings-generate.json",
		"findings-diff.json",
	} {
		if err := verifyEmptyFindings(path, files[path]); err != nil {
			return Bundle{}, err
		}
	}
	for _, path := range []string{"registry-model.json", diffPath} {
		if err := verifyOneJSONValue(path, files[path]); err != nil {
			return Bundle{}, err
		}
	}
	artifacts := make([]Artifact, 0, len(expectedFiles))
	for _, path := range expectedFiles {
		artifacts = append(artifacts, Artifact{Path: path, SHA256: digest(files[path])})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return Bundle{
		Model:            append([]byte(nil), files["registry-model.json"]...),
		Diff:             append([]byte(nil), files[diffPath]...),
		FindingsCheck:    append([]byte(nil), files["findings-check.json"]...),
		FindingsGenerate: append([]byte(nil), files["findings-generate.json"]...),
		FindingsDiff:     append([]byte(nil), files["findings-diff.json"]...),
		Provenance:       append([]byte(nil), files["provenance.json"]...),
		EvidenceSHA256:   digestArtifactList(artifacts),
		Artifacts:        artifacts,
	}, nil
}

func verifyRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("Weaver evidence root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Weaver evidence root must be a non-symlink directory")
	}
	var paths []string
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Weaver evidence symlink %q is forbidden", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("Weaver evidence %q is not a regular file", rel)
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	want := append([]string(nil), expectedFiles...)
	sort.Strings(want)
	if !equalStrings(paths, want) {
		return fmt.Errorf("Weaver evidence file set = %q, want %q", paths, want)
	}
	return nil
}

func readEvidenceFiles(root string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(expectedFiles))
	for _, path := range expectedFiles {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, fmt.Errorf("read Weaver evidence %q: %w", path, err)
		}
		result[path] = data
	}
	return result, nil
}

func verifyProvenance(fixtureRoot string, files map[string][]byte, got provenance) error {
	if got.SchemaVersion != 2 {
		return fmt.Errorf("Weaver provenance schema_version = %d, want 2", got.SchemaVersion)
	}
	if got.Weaver.Release != expectedWeaverRelease || got.Weaver.Commit != expectedWeaverCommit {
		return errors.New("Weaver provenance tool identity mismatch")
	}
	platform, ok := platforms[got.Weaver.Platform]
	if !ok {
		return fmt.Errorf("Weaver provenance platform %q is unsupported", got.Weaver.Platform)
	}
	if got.Weaver.Archive != platform.archive || got.Weaver.ArchiveSHA256 != platform.sum ||
		!validDigest(got.Weaver.ExecutableSHA256) {
		return errors.New("Weaver provenance executable identity mismatch")
	}
	if err := verifyRegistry(
		"source", got.Source,
		expectedSourceTag, expectedSourceCommit, expectedSourceArchive, expectedSourceSHA,
	); err != nil {
		return err
	}
	if err := verifyRegistry(
		"target", got.Target,
		expectedTargetTag, expectedTargetCommit, expectedTargetArchive, expectedTargetSHA,
	); err != nil {
		return err
	}
	if err := verifyLocalInputs(fixtureRoot, got.Inputs); err != nil {
		return err
	}
	if err := verifyCommands(got.Commands, platform); err != nil {
		return err
	}
	if len(got.Artifacts) != len(expectedArtifacts) {
		return fmt.Errorf("Weaver provenance artifact count = %d, want %d", len(got.Artifacts), len(expectedArtifacts))
	}
	for index, path := range expectedArtifacts {
		item := got.Artifacts[index]
		if item.Path != path || item.SHA256 != digest(files[path]) {
			return fmt.Errorf("Weaver provenance artifact %d mismatch for %q", index, path)
		}
	}
	return nil
}

func verifyRegistry(label string, got provenanceRegistry, tag, commit, archive, sum string) error {
	if got.Tag != tag || got.Commit != commit || got.Archive != archive || got.ArchiveSHA256 != sum {
		return fmt.Errorf("Weaver provenance %s Registry identity mismatch", label)
	}
	return nil
}

func verifyLocalInputs(root string, got []Artifact) error {
	if len(got) != len(expectedLocalInputs) {
		return fmt.Errorf("Weaver provenance local input count = %d, want %d", len(got), len(expectedLocalInputs))
	}
	for index, path := range expectedLocalInputs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read Weaver provenance input %q: %w", path, err)
		}
		if got[index].Path != path || got[index].SHA256 != digest(data) {
			return fmt.Errorf("Weaver provenance local input %d mismatch for %q", index, path)
		}
	}
	return nil
}

func verifyCommands(got []command, platform platformPin) error {
	binary := "${STAGING}/tool/" + platform.binary + "/weaver"
	target := "${STAGING}/registry/target/semantic-conventions-v1.41.0/model"
	common := []string{
		"--quiet",
		"--registry", target,
		"--follow-symlinks=false",
		"--include-unreferenced=false",
		"--v2=false",
		"--diagnostic-format=json",
		"--diagnostic-stdout=true",
	}
	check := append([]string{binary, "registry", "check"}, common...)
	check = append(check,
		"--config", "${STAGING}/config/.weaver.toml",
		"--policy", "${STAGING}/local-inputs/policies",
	)
	generate := []string{
		binary, "registry", "generate", "otelmap-weave", "${STAGING}/artifacts",
	}
	generate = append(generate, common...)
	generate = append(generate,
		"--templates", "${STAGING}/local-inputs/templates",
		"--policy", "${STAGING}/local-inputs/policies",
	)
	diff := append([]string{binary, "registry", "diff"}, common...)
	diff = append(diff,
		"--config", "${STAGING}/config/.weaver.toml",
		"--baseline-registry", "${STAGING}/registry/source/semantic-conventions-v1.26.0/model",
		"--format=json",
		"--output", "${STAGING}/artifacts/diff/v1.26.0-to-v1.41.0",
	)
	want := []command{
		{Name: "registry check", Argv: check},
		{Name: "registry generate", Argv: generate},
		{Name: "registry diff", Argv: diff},
	}
	if len(got) != len(want) {
		return fmt.Errorf("Weaver provenance command count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].Name != want[index].Name || !equalStrings(got[index].Argv, want[index].Argv) {
			return fmt.Errorf("Weaver provenance command %d mismatch", index)
		}
	}
	return nil
}

func verifyEmptyFindings(path string, data []byte) error {
	var findings []json.RawMessage
	if err := strictDecode(data, &findings); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if len(findings) != 0 {
		return fmt.Errorf("%s contains %d unresolved findings", path, len(findings))
	}
	return nil
}

func verifyOneJSONValue(path string, data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s has trailing JSON content", path)
	}
	return nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func digestArtifactList(artifacts []Artifact) string {
	hash := sha256.New()
	for _, artifact := range artifacts {
		hash.Write([]byte(artifact.Path))
		hash.Write([]byte{0})
		hash.Write([]byte(artifact.SHA256))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
