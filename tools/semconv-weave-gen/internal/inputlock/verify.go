package inputlock

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	lockFileName     = "input-lock.json"
	manifestFileName = "checksums.txt"

	expectedGoToolchain = "go1.25.12"

	expectedWeaverRelease = "v0.25.0"
	expectedWeaverCommit  = "a177fb605e66ef8d3b333302459c805091bfedd7"

	expectedSourceRegistryTag        = "v1.26.0"
	expectedSourceRegistryCommit     = "9adff435c76ea3b8cba89babefc832d3dd3c1ab9"
	expectedSourceRegistryArchive    = "upstream/registry/semantic-conventions-v1.26.0-model.tar"
	expectedSourceRegistryArchiveSHA = "ecf8658f85d2e06281470ec1d11d27534a6090f6e689343e757ba4ab7e1d1ece"

	expectedTargetRegistryTag        = "v1.41.0"
	expectedTargetRegistryCommit     = "e018fe6f91862f5ed63c082f87697cddac596784"
	expectedTargetRegistryArchive    = "upstream/registry/semantic-conventions-v1.41.0-model.tar"
	expectedTargetRegistryArchiveSHA = "6e2a13d9ccac76327bbb79578eced858b14fddbc96facbf56aab16a5b19cee93"

	expectedSchemaURL  = "https://opentelemetry.io/schemas/1.41.0"
	expectedSchemaPath = "schema/opentelemetry-1.41.0.yaml"
	expectedSchemaSHA  = "d73002442a640ce74a185359843b9046884d16eec198c3576d2dcc779deaf740"

	expectedWeaverManifest    = "upstream/weaver/sha256.sum"
	expectedWeaverManifestSHA = "f528e522d616bf3d385f510f95fd3f5afd06db3481cb1cff6277f2f7c3511776"
)

// Pins is the authoritative pinned upstream identity for the release. It is the
// single place these values are declared; release evidence is checked against
// it rather than against the artifact under verification.
type Pins struct {
	WeaverRelease        string
	WeaverCommit         string
	SourceRegistryTag    string
	SourceRegistryCommit string
	SourceRegistrySHA256 string
	TargetRegistryTag    string
	TargetRegistryCommit string
	TargetRegistrySHA256 string
	SchemaURL            string
	SchemaSHA256         string
}

// PinnedInputs returns the pinned upstream identity enforced by Verify.
func PinnedInputs() Pins {
	return Pins{
		WeaverRelease:        expectedWeaverRelease,
		WeaverCommit:         expectedWeaverCommit,
		SourceRegistryTag:    expectedSourceRegistryTag,
		SourceRegistryCommit: expectedSourceRegistryCommit,
		SourceRegistrySHA256: expectedSourceRegistryArchiveSHA,
		TargetRegistryTag:    expectedTargetRegistryTag,
		TargetRegistryCommit: expectedTargetRegistryCommit,
		TargetRegistrySHA256: expectedTargetRegistryArchiveSHA,
		SchemaURL:            expectedSchemaURL,
		SchemaSHA256:         expectedSchemaSHA,
	}
}

var checksumLine = regexp.MustCompile(`^([0-9a-f]{64})  ([^\r\n]+)$`)

type inputLock struct {
	SchemaVersion    int              `json:"schema_version"`
	GoToolchain      string           `json:"go_toolchain"`
	Weaver           weaverLock       `json:"weaver"`
	SourceRegistry   registryLock     `json:"source_registry"`
	TargetRegistry   registryLock     `json:"target_registry"`
	Schema           schemaLock       `json:"schema"`
	MigrationGuides  []migrationGuide `json:"migration_guides"`
	LocalInputs      []string         `json:"local_inputs"`
	ChecksumManifest string           `json:"checksum_manifest"`
}

type weaverLock struct {
	Release          string        `json:"release"`
	Commit           string        `json:"commit"`
	ChecksumManifest string        `json:"checksum_manifest"`
	Assets           []weaverAsset `json:"assets"`
}

type weaverAsset struct {
	Platform string `json:"platform"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
}

type registryLock struct {
	Tag     string `json:"tag"`
	Commit  string `json:"commit"`
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
}

type schemaLock struct {
	URL    string `json:"url"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type migrationGuide struct {
	Domain string `json:"domain"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type checksumEntry struct {
	path string
	sum  string
}

var expectedWeaverAssets = []weaverAsset{
	{
		Platform: "darwin/arm64",
		Path:     "upstream/weaver/weaver-aarch64-apple-darwin.tar.xz",
		SHA256:   "32f1c4e9092f0ba4fe48b259ce3dc1d65c756db7f759fadf5338ebacdf53a3df",
	},
	{
		Platform: "linux/amd64",
		Path:     "upstream/weaver/weaver-x86_64-unknown-linux-gnu.tar.xz",
		SHA256:   "8cbf46fc0137311eab7fd3860cb841031bb772f8613acc5923fa9ef3879c0cd5",
	},
}

var expectedMigrationGuides = []migrationGuide{
	{
		Domain: "db",
		Path:   "guides/db-migration.md",
		SHA256: "52746f889ec489c988d49a8cbd2ab021d33bff436d81dfff307c0bec7a67e32d",
	},
	{
		Domain: "http",
		Path:   "guides/http-migration.md",
		SHA256: "2726327180ab7910de371ecdc65dc52bcd10c09b24ea39800408024fe63fe4b1",
	},
	{
		Domain: "rpc",
		Path:   "guides/rpc-migration.md",
		SHA256: "30a1b88fae6dc349f6ca5bb3095b2c7cd4f793bd63601f234c68e504faaa306b",
	},
}

var expectedLocalInputs = []string{
	"audit/compatibility-matrix.json",
	"audit/db-rpc-decisions.json",
	"audit/db-rpc-obligations.json",
	"audit/http-url-network-decisions.json",
	"audit/http-url-network-obligations.json",
	"audit/migration-audit-v1.41.0.yaml",
	"audit/required-source-urls-draft.json",
	"audit/required-source-urls.yaml",
	"corpus/canonical-technology-aliases.json",
	"corpus/collector-schema-urls.json",
	"local/policies/otelmap-canonical.rego",
	"local/templates/registry/otelmap-weave/registry-model.json.j2",
	"local/templates/registry/otelmap-weave/registry-review.md.j2",
	"local/templates/registry/otelmap-weave/weaver.yaml",
}

// Verify checks the complete pinned input closure rooted at root without
// following symlinks, modifying files, or accessing the network.
func Verify(root string) error {
	if root == "" {
		return errors.New("fixture root is required")
	}
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("fixture root %q: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("fixture root %q is a symlink", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("fixture root %q is not a directory", root)
	}

	manifestPath := filepath.Join(root, manifestFileName)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read checksum manifest %q: %w", manifestPath, err)
	}
	entries, err := parseChecksumManifest(manifestBytes)
	if err != nil {
		return fmt.Errorf("parse checksum manifest %q: %w", manifestPath, err)
	}
	files, err := collectFixtureFiles(root)
	if err != nil {
		return err
	}
	if err := verifyFileSet(entries, files); err != nil {
		return err
	}
	if err := verifyChecksums(root, entries); err != nil {
		return err
	}

	lock, err := decodeLock(filepath.Join(root, lockFileName))
	if err != nil {
		return err
	}
	if err := verifyIdentity(lock, entries); err != nil {
		return err
	}
	return nil
}

func parseChecksumManifest(data []byte) ([]checksumEntry, error) {
	if len(data) == 0 {
		return nil, errors.New("checksum manifest is empty")
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("checksum manifest must end with LF")
	}

	var entries []checksumEntry
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()
		match := checksumLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("line %d: expected lowercase SHA-256, two spaces, and POSIX path", lineNumber)
		}
		rel := match[2]
		if err := validateRelativePath(rel); err != nil {
			return nil, fmt.Errorf("line %d: unsafe checksum path %q: %w", lineNumber, rel, err)
		}
		if rel == manifestFileName {
			return nil, fmt.Errorf("line %d: checksum manifest cannot reference itself", lineNumber)
		}
		if _, ok := seen[rel]; ok {
			return nil, fmt.Errorf("line %d: duplicate checksum path %q", lineNumber, rel)
		}
		if len(entries) > 0 && rel < entries[len(entries)-1].path {
			return nil, fmt.Errorf("line %d: checksum paths are not sorted: %q precedes %q", lineNumber, rel, entries[len(entries)-1].path)
		}
		seen[rel] = struct{}{}
		entries = append(entries, checksumEntry{path: rel, sum: match[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksum manifest: %w", err)
	}
	return entries, nil
}

func validateRelativePath(rel string) error {
	if rel == "" || strings.Contains(rel, `\`) {
		return errors.New("path must use non-empty POSIX syntax")
	}
	if path.IsAbs(rel) || path.Clean(rel) != rel || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return errors.New("path must be clean and relative")
	}
	return nil
}

func collectFixtureFiles(root string) (map[string]struct{}, error) {
	files := make(map[string]struct{})
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
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
			return fmt.Errorf("fixture symlink %q is not allowed", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("fixture %q is not a regular file", rel)
		}
		if rel != manifestFileName {
			files[rel] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk fixture root %q: %w", root, err)
	}
	return files, nil
}

func verifyFileSet(entries []checksumEntry, files map[string]struct{}) error {
	listed := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		listed[entry.path] = struct{}{}
		if _, ok := files[entry.path]; !ok {
			return fmt.Errorf("missing fixture %q listed by checksum manifest", entry.path)
		}
	}
	var extras []string
	for rel := range files {
		if _, ok := listed[rel]; !ok {
			extras = append(extras, rel)
		}
	}
	if len(extras) > 0 {
		sort.Strings(extras)
		return fmt.Errorf("unlisted fixture %q", extras[0])
	}
	return nil
}

func verifyChecksums(root string, entries []checksumEntry) error {
	for _, entry := range entries {
		filePath := filepath.Join(root, filepath.FromSlash(entry.path))
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open fixture %q: %w", entry.path, err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash fixture %q: %w", entry.path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close fixture %q: %w", entry.path, closeErr)
		}
		actual := hex.EncodeToString(hash.Sum(nil))
		if actual != entry.sum {
			return fmt.Errorf("checksum mismatch for %q: got %s, want %s", entry.path, actual, entry.sum)
		}
	}
	return nil
}

func decodeLock(lockPath string) (inputLock, error) {
	file, err := os.Open(lockPath)
	if err != nil {
		return inputLock{}, fmt.Errorf("open input lock %q: %w", lockPath, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var lock inputLock
	if err := decoder.Decode(&lock); err != nil {
		return inputLock{}, fmt.Errorf("decode input lock %q: %w", lockPath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return inputLock{}, fmt.Errorf("decode input lock %q: trailing JSON value", lockPath)
		}
		return inputLock{}, fmt.Errorf("decode input lock %q: %w", lockPath, err)
	}
	return lock, nil
}

func verifyIdentity(lock inputLock, entries []checksumEntry) error {
	if lock.SchemaVersion != 2 {
		return fmt.Errorf("input lock schema_version: got %d, want 2", lock.SchemaVersion)
	}
	if lock.GoToolchain != expectedGoToolchain {
		return fmt.Errorf("go toolchain: got %q, want %q", lock.GoToolchain, expectedGoToolchain)
	}
	if lock.ChecksumManifest != manifestFileName {
		return fmt.Errorf("checksum_manifest: got %q, want %q", lock.ChecksumManifest, manifestFileName)
	}
	if lock.Weaver.Release != expectedWeaverRelease {
		return fmt.Errorf("weaver release: got %q, want %q", lock.Weaver.Release, expectedWeaverRelease)
	}
	if lock.Weaver.Commit != expectedWeaverCommit {
		return fmt.Errorf("weaver commit: got %q, want %q", lock.Weaver.Commit, expectedWeaverCommit)
	}
	if lock.Weaver.ChecksumManifest != expectedWeaverManifest {
		return fmt.Errorf("weaver checksum manifest: got %q, want %q", lock.Weaver.ChecksumManifest, expectedWeaverManifest)
	}
	if err := equalWeaverAssets(lock.Weaver.Assets, expectedWeaverAssets); err != nil {
		return err
	}
	if err := verifyRegistryIdentity(
		"source registry", lock.SourceRegistry,
		expectedSourceRegistryTag, expectedSourceRegistryCommit,
		expectedSourceRegistryArchive, expectedSourceRegistryArchiveSHA,
	); err != nil {
		return err
	}
	if err := verifyRegistryIdentity(
		"target registry", lock.TargetRegistry,
		expectedTargetRegistryTag, expectedTargetRegistryCommit,
		expectedTargetRegistryArchive, expectedTargetRegistryArchiveSHA,
	); err != nil {
		return err
	}
	if lock.Schema.URL != expectedSchemaURL {
		return fmt.Errorf("schema URL: got %q, want %q", lock.Schema.URL, expectedSchemaURL)
	}
	if lock.Schema.Path != expectedSchemaPath {
		return fmt.Errorf("schema path: got %q, want %q", lock.Schema.Path, expectedSchemaPath)
	}
	if lock.Schema.SHA256 != expectedSchemaSHA {
		return fmt.Errorf("schema sha256: got %q, want %q", lock.Schema.SHA256, expectedSchemaSHA)
	}
	if err := equalMigrationGuides(lock.MigrationGuides, expectedMigrationGuides); err != nil {
		return err
	}
	if !equalStrings(lock.LocalInputs, expectedLocalInputs) {
		return fmt.Errorf("local_inputs: got %q, want %q", lock.LocalInputs, expectedLocalInputs)
	}

	sums := make(map[string]string, len(entries))
	for _, entry := range entries {
		sums[entry.path] = entry.sum
	}
	required := map[string]string{
		expectedWeaverManifest:        expectedWeaverManifestSHA,
		expectedSourceRegistryArchive: expectedSourceRegistryArchiveSHA,
		expectedTargetRegistryArchive: expectedTargetRegistryArchiveSHA,
		expectedSchemaPath:            expectedSchemaSHA,
	}
	for _, asset := range expectedWeaverAssets {
		required[asset.Path] = asset.SHA256
	}
	for _, guide := range expectedMigrationGuides {
		required[guide.Path] = guide.SHA256
	}
	for rel, want := range required {
		if got := sums[rel]; got != want {
			return fmt.Errorf("pinned checksum for %q: got %q, want %q", rel, got, want)
		}
	}
	return nil
}

func verifyRegistryIdentity(
	label string,
	got registryLock,
	tag, commit, archive, sum string,
) error {
	if got.Tag != tag {
		return fmt.Errorf("%s tag: got %q, want %q", label, got.Tag, tag)
	}
	if got.Commit != commit {
		return fmt.Errorf("%s commit: got %q, want %q", label, got.Commit, commit)
	}
	if got.Archive != archive {
		return fmt.Errorf("%s archive: got %q, want %q", label, got.Archive, archive)
	}
	if got.SHA256 != sum {
		return fmt.Errorf("%s sha256: got %q, want %q", label, got.SHA256, sum)
	}
	return nil
}

func equalWeaverAssets(got, want []weaverAsset) error {
	if len(got) != len(want) {
		return fmt.Errorf("weaver assets: got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("weaver asset %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	return nil
}

func equalMigrationGuides(got, want []migrationGuide) error {
	if len(got) != len(want) {
		return fmt.Errorf("migration guides: got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("migration guide %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	return nil
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
