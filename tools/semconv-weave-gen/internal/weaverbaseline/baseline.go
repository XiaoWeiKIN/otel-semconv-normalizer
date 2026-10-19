package weaverbaseline

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/inputlock"
)

const (
	weaverRelease = "v0.25.0"
	weaverCommit  = "a177fb605e66ef8d3b333302459c805091bfedd7"

	sourceRegistryTag        = "v1.26.0"
	sourceRegistryCommit     = "9adff435c76ea3b8cba89babefc832d3dd3c1ab9"
	sourceRegistryArchiveRel = "upstream/registry/semantic-conventions-v1.26.0-model.tar"
	sourceRegistryArchiveSHA = "ecf8658f85d2e06281470ec1d11d27534a6090f6e689343e757ba4ab7e1d1ece"
	sourceRegistryRoot       = "semantic-conventions-v1.26.0/model"

	targetRegistryTag        = "v1.41.0"
	targetRegistryCommit     = "e018fe6f91862f5ed63c082f87697cddac596784"
	targetRegistryArchiveRel = "upstream/registry/semantic-conventions-v1.41.0-model.tar"
	targetRegistryArchiveSHA = "6e2a13d9ccac76327bbb79578eced858b14fddbc96facbf56aab16a5b19cee93"
	targetRegistryRoot       = "semantic-conventions-v1.41.0/model"

	diffInterval = "v1.26.0-to-v1.41.0"

	policyRel    = "local/policies"
	templatesRel = "local/templates"
)

type Config struct {
	Inputs string
	Output string

	GOOS   string
	GOARCH string
	Tar    string
}

type commandKind string

const (
	commandCheck    commandKind = "check"
	commandGenerate commandKind = "generate"
	commandDiff     commandKind = "diff"
)

type commandPaths struct {
	weaver         string
	sourceRegistry string
	targetRegistry string
	policy         string
	templates      string
	projectConfig  string
	output         string
	diff           string
}

type commandSpec struct {
	kind commandKind
	args []string
}

type platformAsset struct {
	Platform string
	Path     string
	SHA256   string
}

var platformAssets = []platformAsset{
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

type provenance struct {
	SchemaVersion int                  `json:"schema_version"`
	Weaver        provenanceWeaver     `json:"weaver"`
	Source        provenanceRegistry   `json:"source_registry"`
	Target        provenanceRegistry   `json:"target_registry"`
	Inputs        []provenanceArtifact `json:"local_inputs"`
	Commands      []provenanceCommand  `json:"commands"`
	Artifacts     []provenanceArtifact `json:"artifacts"`
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

type provenanceCommand struct {
	Name string   `json:"name"`
	Argv []string `json:"argv"`
}

type provenanceArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func Run(ctx context.Context, config Config) error {
	cfg, err := normalizeConfig(config)
	if err != nil {
		return err
	}
	if err := inputlock.Verify(cfg.Inputs); err != nil {
		return fmt.Errorf("verify pinned inputs: %w", err)
	}
	asset, err := selectAsset(cfg.GOOS, cfg.GOARCH)
	if err != nil {
		return err
	}

	parent := filepath.Dir(cfg.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".weaver-baseline-stage-")
	if err != nil {
		return fmt.Errorf("create staging root: %w", err)
	}
	defer os.RemoveAll(stage)

	first := filepath.Join(stage, "run-1")
	second := filepath.Join(stage, "run-2")
	if err := runOnce(ctx, cfg, asset, first); err != nil {
		return fmt.Errorf("first Weaver pipeline: %w", err)
	}
	if err := runOnce(ctx, cfg, asset, second); err != nil {
		return fmt.Errorf("second Weaver pipeline: %w", err)
	}
	firstArtifacts := filepath.Join(first, "artifacts")
	secondArtifacts := filepath.Join(second, "artifacts")
	if err := CompareTrees(firstArtifacts, secondArtifacts); err != nil {
		return fmt.Errorf("Weaver pipeline is not byte-identical: %w", err)
	}
	if err := promoteTree(firstArtifacts, cfg.Output); err != nil {
		return fmt.Errorf("promote Weaver artifacts: %w", err)
	}
	return nil
}

func normalizeConfig(config Config) (Config, error) {
	if config.Inputs == "" {
		return Config{}, errors.New("inputs path is required")
	}
	if config.Output == "" {
		return Config{}, errors.New("output path is required")
	}
	inputs, err := filepath.Abs(config.Inputs)
	if err != nil {
		return Config{}, fmt.Errorf("absolute inputs path: %w", err)
	}
	output, err := filepath.Abs(config.Output)
	if err != nil {
		return Config{}, fmt.Errorf("absolute output path: %w", err)
	}
	inputs = filepath.Clean(inputs)
	output = filepath.Clean(output)
	// promoteTree renames the whole output tree away before swapping the staged
	// artifacts in, so an output that contains the inputs would destroy them.
	// Compare resolved paths as well: a symlinked prefix would otherwise let an
	// output that really is an ancestor of inputs past a lexical check.
	if overlaps(output, inputs) {
		return Config{}, errors.New("output must not overlap the checksum-locked inputs tree")
	}
	if info, err := os.Lstat(inputs); err != nil {
		return Config{}, fmt.Errorf("inputs path: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return Config{}, errors.New("inputs path must not be a symlink")
	}
	if config.GOOS == "" {
		config.GOOS = runtime.GOOS
	}
	if config.GOARCH == "" {
		config.GOARCH = runtime.GOARCH
	}
	if config.Tar == "" {
		config.Tar, err = exec.LookPath("tar")
		if err != nil {
			return Config{}, errors.New("tar executable is required to extract the checksum-verified Weaver .tar.xz asset")
		}
	}
	config.Tar, err = filepath.Abs(config.Tar)
	if err != nil {
		return Config{}, fmt.Errorf("absolute tar path: %w", err)
	}
	config.Inputs = inputs
	config.Output = output
	return config, nil
}

func within(candidate, root string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// overlaps reports whether either path contains the other, comparing both the
// literal and the symlink-resolved forms. Neither path need exist; the nearest
// existing ancestor is resolved and the unresolved remainder appended.
func overlaps(left, right string) bool {
	if within(left, right) || within(right, left) {
		return true
	}
	resolvedLeft, resolvedRight := resolveExisting(left), resolveExisting(right)
	return within(resolvedLeft, resolvedRight) || within(resolvedRight, resolvedLeft)
}

func resolveExisting(path string) string {
	remainder := ""
	for current := path; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

func selectAsset(goos, goarch string) (platformAsset, error) {
	platform := goos + "/" + goarch
	for _, asset := range platformAssets {
		if asset.Platform == platform {
			return asset, nil
		}
	}
	return platformAsset{}, fmt.Errorf("unsupported Weaver platform %q", platform)
}

func runOnce(ctx context.Context, cfg Config, asset platformAsset, root string) error {
	for _, dir := range []string{
		root,
		filepath.Join(root, "home"),
		filepath.Join(root, "config"),
		filepath.Join(root, "work"),
		filepath.Join(root, "artifacts"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	weaver, executableSHA, err := materializeWeaver(ctx, cfg, asset, filepath.Join(root, "tool"))
	if err != nil {
		return err
	}
	sourceRegistry, err := materializeRegistry(
		cfg.Inputs,
		filepath.Join(root, "registry", "source"),
		registryFixture{
			archive: sourceRegistryArchiveRel,
			root:    sourceRegistryRoot,
		},
	)
	if err != nil {
		return err
	}
	targetRegistry, err := materializeRegistry(
		cfg.Inputs,
		filepath.Join(root, "registry", "target"),
		registryFixture{
			archive: targetRegistryArchiveRel,
			root:    targetRegistryRoot,
		},
	)
	if err != nil {
		return err
	}
	artifacts := filepath.Join(root, "artifacts")
	projectConfig := filepath.Join(root, "config", ".weaver.toml")
	if err := os.WriteFile(projectConfig, nil, 0o644); err != nil {
		return fmt.Errorf("write isolated Weaver project config: %w", err)
	}
	materializedInputs := filepath.Join(root, "local-inputs")
	policy := filepath.Join(materializedInputs, "policies")
	templates := filepath.Join(materializedInputs, "templates")
	if err := copyTree(filepath.Join(cfg.Inputs, filepath.FromSlash(policyRel)), policy); err != nil {
		return fmt.Errorf("materialize policy: %w", err)
	}
	if err := copyTree(filepath.Join(cfg.Inputs, filepath.FromSlash(templatesRel)), templates); err != nil {
		return fmt.Errorf("materialize templates: %w", err)
	}
	paths := commandPaths{
		weaver:         weaver,
		sourceRegistry: sourceRegistry,
		targetRegistry: targetRegistry,
		policy:         policy,
		templates:      templates,
		projectConfig:  projectConfig,
		output:         artifacts,
		diff:           filepath.Join(artifacts, "diff", diffInterval),
	}
	specs := commandSpecifications(paths)
	env := isolatedEnv(filepath.Join(root, "home"), filepath.Join(root, "config"))
	work := filepath.Join(root, "work")
	for _, spec := range specs {
		if err := validateCommand(spec); err != nil {
			return err
		}
		if spec.kind == commandDiff {
			if err := os.MkdirAll(paths.diff, 0o755); err != nil {
				return err
			}
		}
		stdout, stderr, err := runCommand(ctx, work, env, paths.weaver, spec.args)
		diagnostics, err := normalizeProcessOutput(stdout, stderr, err)
		if err != nil {
			return fmt.Errorf("weaver registry %s diagnostics: %w", spec.kind, err)
		}
		name := "findings-" + string(spec.kind) + ".json"
		if err := os.WriteFile(filepath.Join(artifacts, name), diagnostics, 0o644); err != nil {
			return err
		}
	}
	for _, required := range []string{
		"registry-model.json",
		"registry-review.md",
		"diff/" + diffInterval + "/diff.json",
	} {
		if info, err := os.Stat(filepath.Join(artifacts, filepath.FromSlash(required))); err != nil {
			return fmt.Errorf("required Weaver artifact %q: %w", required, err)
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("required Weaver artifact %q is not regular", required)
		}
	}
	for _, jsonArtifact := range []string{
		"registry-model.json",
		"diff/" + diffInterval + "/diff.json",
	} {
		replacements := map[string]string(nil)
		if jsonArtifact == "registry-model.json" {
			replacements = map[string]string{
				targetRegistry: targetRegistryRoot,
			}
		}
		normalizer := func(any) error { return nil }
		if strings.HasPrefix(jsonArtifact, "diff/") {
			normalizer = sortWeaverDiffChanges
		}
		if err := canonicalizeJSONFile(
			filepath.Join(artifacts, filepath.FromSlash(jsonArtifact)),
			replacements,
			normalizer,
		); err != nil {
			return fmt.Errorf("canonicalize Weaver artifact %q: %w", jsonArtifact, err)
		}
	}
	if err := writeProvenance(artifacts, root, cfg.Inputs, paths.weaver, asset, executableSHA, specs); err != nil {
		return err
	}
	return nil
}

func materializeWeaver(ctx context.Context, cfg Config, asset platformAsset, dst string) (string, string, error) {
	archive := filepath.Join(cfg.Inputs, filepath.FromSlash(asset.Path))
	listed, stderr, err := runCommand(ctx, dst, isolatedEnv(filepath.Join(dst, "home"), filepath.Join(dst, "config")), cfg.Tar, []string{"-tJf", archive})
	if err != nil {
		return "", "", fmt.Errorf("list Weaver archive: %w", err)
	}
	if len(bytes.TrimSpace(stderr)) != 0 {
		return "", "", fmt.Errorf("list Weaver archive wrote stderr: %q", stderr)
	}
	if err := validateArchivePaths(listed); err != nil {
		return "", "", fmt.Errorf("unsafe Weaver archive: %w", err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", "", err
	}
	_, stderr, err = runCommand(ctx, dst, isolatedEnv(filepath.Join(dst, "home"), filepath.Join(dst, "config")), cfg.Tar, []string{"-xJf", archive, "-C", dst})
	if err != nil {
		return "", "", fmt.Errorf("extract Weaver archive: %w", err)
	}
	if len(bytes.TrimSpace(stderr)) != 0 {
		return "", "", fmt.Errorf("extract Weaver archive wrote stderr: %q", stderr)
	}
	var candidates []string
	err = filepath.WalkDir(dst, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("extracted symlink %q is forbidden", current)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("extracted non-regular file %q is forbidden", current)
		}
		if entry.Name() == "weaver" {
			candidates = append(candidates, current)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if len(candidates) != 1 {
		return "", "", fmt.Errorf("Weaver archive contains %d executables named weaver, want 1", len(candidates))
	}
	weaver, err := filepath.Abs(candidates[0])
	if err != nil {
		return "", "", err
	}
	version, versionErr, err := runCommand(ctx, dst, isolatedEnv(filepath.Join(dst, "home"), filepath.Join(dst, "config")), weaver, []string{"--version"})
	if err != nil {
		return "", "", fmt.Errorf("weaver --version: %w", err)
	}
	if len(bytes.TrimSpace(versionErr)) != 0 {
		return "", "", fmt.Errorf("weaver --version wrote stderr: %q", versionErr)
	}
	if string(bytes.TrimSpace(version)) != "weaver 0.25.0" {
		return "", "", fmt.Errorf("unexpected Weaver version %q", bytes.TrimSpace(version))
	}
	sum, err := fileSHA256(weaver)
	if err != nil {
		return "", "", err
	}
	return weaver, sum, nil
}

func validateArchivePaths(listing []byte) error {
	lines := strings.Split(strings.TrimSuffix(string(listing), "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return errors.New("archive is empty")
	}
	for _, entry := range lines {
		if entry == "" || strings.Contains(entry, `\`) {
			return fmt.Errorf("unsafe archive path %q", entry)
		}
		cleaned := path.Clean(entry)
		if path.IsAbs(entry) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return fmt.Errorf("unsafe archive path %q", entry)
		}
	}
	return nil
}

type registryFixture struct {
	archive string
	root    string
}

func materializeRegistry(inputs, dst string, fixture registryFixture) (string, error) {
	archivePath := filepath.Join(inputs, filepath.FromSlash(fixture.archive))
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	if err := extractRegistryArchive(reader, dst); err != nil {
		return "", err
	}
	root := filepath.Join(dst, filepath.FromSlash(fixture.root))
	if info, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("Registry root: %w", err)
	} else if !info.IsDir() {
		return "", errors.New("Registry root is not a directory")
	}
	return filepath.Abs(root)
}

func extractRegistryArchive(reader *tar.Reader, dst string) error {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read Registry archive: %w", err)
		}
		name := path.Clean(header.Name)
		if path.IsAbs(header.Name) || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(header.Name, `\`) {
			return fmt.Errorf("unsafe Registry archive path %q", header.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if !within(target, dst) {
			return fmt.Errorf("Registry archive path escapes destination: %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			// git archive may emit POSIX PAX metadata records. They contain
			// metadata only and do not materialize a filesystem entry.
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("Registry archive entry %q has forbidden type %d", header.Name, header.Typeflag)
		}
	}
	return nil
}

func commandSpecifications(paths commandPaths) []commandSpec {
	common := []string{
		"--quiet",
		"--registry", paths.targetRegistry,
		"--follow-symlinks=false",
		"--include-unreferenced=false",
		"--v2=false",
		"--diagnostic-format=json",
		"--diagnostic-stdout=true",
	}
	check := append([]string{"registry", "check"}, common...)
	check = append(check, "--config", paths.projectConfig, "--policy", paths.policy)
	generate := []string{"registry", "generate", "otelmap-weave", paths.output}
	generate = append(generate, common...)
	generate = append(generate,
		"--templates", paths.templates,
		"--policy", paths.policy,
	)
	diff := []string{"registry", "diff"}
	diff = append(diff, common...)
	diff = append(diff,
		"--config", paths.projectConfig,
		"--baseline-registry", paths.sourceRegistry,
		"--format=json",
		"--output", paths.diff,
	)
	return []commandSpec{
		{kind: commandCheck, args: check},
		{kind: commandGenerate, args: generate},
		{kind: commandDiff, args: diff},
	}
}

func validateCommand(spec commandSpec) error {
	if len(spec.args) < 2 || spec.args[0] != "registry" {
		return errors.New("Weaver command must begin with registry")
	}
	allowed := map[commandKind]string{
		commandCheck:    "check",
		commandGenerate: "generate",
		commandDiff:     "diff",
	}
	subcommand, ok := allowed[spec.kind]
	if !ok || spec.args[1] != subcommand {
		return fmt.Errorf("forbidden Weaver command kind %q with argv %q", spec.kind, spec.args)
	}
	for _, arg := range spec.args[:2] {
		if arg == "resolve" {
			return errors.New("registry resolve is forbidden")
		}
	}
	return nil
}

func runCommand(ctx context.Context, work string, env []string, executable string, args []string) ([]byte, []byte, error) {
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = work
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("%w; stdout=%q stderr=%q", err, stdout.Bytes(), stderr.Bytes())
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func isolatedEnv(home, config string) []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM":
			continue
		}
		env = append(env, item)
	}
	return append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+config,
		"GIT_CONFIG_GLOBAL="+filepath.Join(config, "empty-gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func NormalizeDiagnostics(data []byte) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return []byte("[]\n"), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode diagnostics: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("trailing diagnostics JSON value")
		}
		return nil, fmt.Errorf("trailing diagnostics content: %w", err)
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("diagnostics must be a JSON array")
	}
	var array []json.RawMessage
	if err := json.Unmarshal(trimmed, &array); err != nil {
		return nil, fmt.Errorf("decode diagnostics JSON array: %w", err)
	}
	normalized, err := json.Marshal(array)
	if err != nil {
		return nil, err
	}
	return append(normalized, '\n'), nil
}

func normalizeProcessOutput(stdout, stderr []byte, runErr error) ([]byte, error) {
	if runErr != nil {
		return nil, fmt.Errorf("process failed: %w", runErr)
	}
	if len(bytes.TrimSpace(stderr)) != 0 {
		return nil, fmt.Errorf("process wrote stderr: %q", stderr)
	}
	return NormalizeDiagnostics(stdout)
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func canonicalizeJSONFile(
	file string,
	replacements map[string]string,
	normalize func(any) error,
) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	normalizeJSONStrings(value, replacements)
	if err := normalize(value); err != nil {
		return err
	}
	canonical, err := canonicalJSON(value)
	if err != nil {
		return err
	}
	return os.WriteFile(file, canonical, 0o644)
}

func sortWeaverDiffChanges(value any) error {
	document, ok := value.(map[string]any)
	if !ok {
		return errors.New("Weaver diff must be a JSON object")
	}
	rawChanges, ok := document["changes"]
	if !ok {
		return errors.New("Weaver diff lacks changes")
	}
	changes, ok := rawChanges.(map[string]any)
	if !ok {
		return errors.New("Weaver diff changes must be an object")
	}
	for category, rawItems := range changes {
		items, ok := rawItems.([]any)
		if !ok {
			return fmt.Errorf("Weaver diff changes.%s must be an array", category)
		}
		type sortableChange struct {
			key   string
			value any
		}
		sorted := make([]sortableChange, len(items))
		for index, item := range items {
			change, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("Weaver diff changes.%s[%d] must be an object", category, index)
			}
			if _, ok := change["type"].(string); !ok {
				return fmt.Errorf("Weaver diff changes.%s[%d] lacks type", category, index)
			}
			data, err := json.Marshal(change)
			if err != nil {
				return err
			}
			sorted[index] = sortableChange{key: string(data), value: item}
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].key < sorted[j].key
		})
		for index := range sorted {
			items[index] = sorted[index].value
			if index > 0 && sorted[index-1].key == sorted[index].key {
				return fmt.Errorf("Weaver diff changes.%s contains a duplicate candidate", category)
			}
		}
	}
	return nil
}

func normalizeJSONStrings(value any, replacements map[string]string) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if text, ok := child.(string); ok {
				for source, target := range replacements {
					if text == source {
						text = target
						break
					}
					if strings.HasPrefix(text, source+string(filepath.Separator)) {
						rel, err := filepath.Rel(source, text)
						if err == nil {
							text = target + "/" + filepath.ToSlash(rel)
						}
						break
					}
				}
				node[key] = text
				continue
			}
			normalizeJSONStrings(child, replacements)
		}
	case []any:
		for _, child := range node {
			normalizeJSONStrings(child, replacements)
		}
	}
}

func writeProvenance(artifacts, runRoot, inputs, weaver string, asset platformAsset, executableSHA string, specs []commandSpec) error {
	files, err := artifactDigests(artifacts)
	if err != nil {
		return err
	}
	localInputs, err := digestRelativeFiles(inputs, []string{
		"checksums.txt",
		"input-lock.json",
		"local/policies/otelmap-canonical.rego",
		"local/templates/registry/otelmap-weave/registry-model.json.j2",
		"local/templates/registry/otelmap-weave/registry-review.md.j2",
		"local/templates/registry/otelmap-weave/weaver.yaml",
	})
	if err != nil {
		return err
	}
	commands := make([]provenanceCommand, 0, len(specs))
	for _, spec := range specs {
		argv := []string{stableArg(weaver, runRoot, inputs)}
		for _, arg := range spec.args {
			argv = append(argv, stableArg(arg, runRoot, inputs))
		}
		commands = append(commands, provenanceCommand{
			Name: "registry " + string(spec.kind),
			Argv: argv,
		})
	}
	document := provenance{
		SchemaVersion: 2,
		Weaver: provenanceWeaver{
			Release:          weaverRelease,
			Commit:           weaverCommit,
			Platform:         asset.Platform,
			Archive:          asset.Path,
			ArchiveSHA256:    asset.SHA256,
			ExecutableSHA256: executableSHA,
		},
		Source: provenanceRegistry{
			Tag:           sourceRegistryTag,
			Commit:        sourceRegistryCommit,
			Archive:       sourceRegistryArchiveRel,
			ArchiveSHA256: sourceRegistryArchiveSHA,
		},
		Target: provenanceRegistry{
			Tag:           targetRegistryTag,
			Commit:        targetRegistryCommit,
			Archive:       targetRegistryArchiveRel,
			ArchiveSHA256: targetRegistryArchiveSHA,
		},
		Inputs:    localInputs,
		Commands:  commands,
		Artifacts: files,
	}
	data, err := canonicalJSON(document)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(artifacts, "provenance.json"), data, 0o644)
}

func digestRelativeFiles(root string, paths []string) ([]provenanceArtifact, error) {
	artifacts := make([]provenanceArtifact, 0, len(paths))
	for _, rel := range paths {
		sum, err := fileSHA256(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("digest %q: %w", rel, err)
		}
		artifacts = append(artifacts, provenanceArtifact{Path: rel, SHA256: sum})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func stableArg(arg, runRoot, inputs string) string {
	for _, replacement := range []struct {
		root  string
		label string
	}{
		{root: runRoot, label: "${STAGING}"},
		{root: inputs, label: "${INPUTS}"},
	} {
		if arg == replacement.root {
			return replacement.label
		}
		if strings.HasPrefix(arg, replacement.root+string(filepath.Separator)) {
			rel, _ := filepath.Rel(replacement.root, arg)
			return replacement.label + "/" + filepath.ToSlash(rel)
		}
	}
	return arg
}

func artifactDigests(root string) ([]provenanceArtifact, error) {
	var artifacts []provenanceArtifact
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("artifact %q is not a regular file", current)
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "provenance.json" {
			return nil
		}
		sum, err := fileSHA256(current)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, provenanceArtifact{
			Path:   filepath.ToSlash(rel),
			SHA256: sum,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func fileSHA256(file string) (string, error) {
	handle, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func CompareTrees(left, right string) error {
	leftFiles, err := treeFiles(left)
	if err != nil {
		return err
	}
	rightFiles, err := treeFiles(right)
	if err != nil {
		return err
	}
	if len(leftFiles) != len(rightFiles) {
		return fmt.Errorf("file count differs: %d != %d", len(leftFiles), len(rightFiles))
	}
	for i, rel := range leftFiles {
		if rightFiles[i] != rel {
			return fmt.Errorf("file set differs at %q != %q", rel, rightFiles[i])
		}
		leftData, err := os.ReadFile(filepath.Join(left, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		rightData, err := os.ReadFile(filepath.Join(right, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if !bytes.Equal(leftData, rightData) {
			return fmt.Errorf("artifact %q bytes differ", rel)
		}
	}
	return nil
}

func treeFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("tree entry %q is not a regular file", current)
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func promoteTree(source, target string) error {
	parent := filepath.Dir(target)
	incoming, err := os.MkdirTemp(parent, ".weaver-baseline-incoming-")
	if err != nil {
		return err
	}
	if err := os.Remove(incoming); err != nil {
		return err
	}
	if err := copyTree(source, incoming); err != nil {
		os.RemoveAll(incoming)
		return err
	}
	backup := ""
	if _, err := os.Lstat(target); err == nil {
		backup, err = os.MkdirTemp(parent, ".weaver-baseline-backup-")
		if err != nil {
			os.RemoveAll(incoming)
			return err
		}
		if err := os.Remove(backup); err != nil {
			os.RemoveAll(incoming)
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			os.RemoveAll(incoming)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		os.RemoveAll(incoming)
		return err
	}
	if err := os.Rename(incoming, target); err != nil {
		if backup != "" {
			_ = os.Rename(backup, target)
		}
		os.RemoveAll(incoming)
		return err
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("source entry %q is not a regular file", current)
		}
		in, err := os.Open(current)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		inCloseErr := in.Close()
		outCloseErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inCloseErr != nil {
			return inCloseErr
		}
		return outCloseErr
	})
}
