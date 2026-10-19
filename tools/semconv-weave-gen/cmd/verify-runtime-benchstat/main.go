// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/perf/benchmath"
)

type options struct {
	baselinePath  string
	candidatePath string
	alpha         float64
	maxRegression float64
	minSamples    int
	prefix        string
}

type benchmarkData struct {
	Config  map[string]string
	Samples map[string]*metricSamples
}

type metricSamples struct {
	TimeNS []float64
	Bytes  []float64
	Allocs []float64
}

type report struct {
	SchemaVersion int               `json:"schema_version"`
	Status        string            `json:"status"`
	Alpha         float64           `json:"alpha"`
	MaxRegression float64           `json:"max_regression"`
	MinSamples    int               `json:"min_samples"`
	Config        map[string]string `json:"config"`
	Comparisons   []benchmarkReport `json:"comparisons"`
}

type benchmarkReport struct {
	Benchmark          string   `json:"benchmark"`
	BaselineSamples    int      `json:"baseline_samples"`
	CandidateSamples   int      `json:"candidate_samples"`
	BaselineMedianNS   float64  `json:"baseline_median_ns"`
	CandidateMedianNS  float64  `json:"candidate_median_ns"`
	Delta              float64  `json:"delta"`
	PValue             float64  `json:"p_value"`
	Significant        bool     `json:"significant"`
	BaselineMaxBytes   float64  `json:"baseline_max_bytes_per_op"`
	CandidateMaxBytes  float64  `json:"candidate_max_bytes_per_op"`
	BaselineMaxAllocs  float64  `json:"baseline_max_allocs_per_op"`
	CandidateMaxAllocs float64  `json:"candidate_max_allocs_per_op"`
	Status             string   `json:"status"`
	Reasons            []string `json:"reasons,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
}

func main() {
	var config options
	flag.StringVar(&config.baselinePath, "baseline", "", "baseline Go benchmark output")
	flag.StringVar(&config.candidatePath, "candidate", "", "candidate Go benchmark output")
	flag.Float64Var(&config.alpha, "alpha", 0.05, "Mann-Whitney significance alpha")
	flag.Float64Var(&config.maxRegression, "max-regression", 0.05, "maximum allowed median ns/op regression")
	flag.IntVar(&config.minSamples, "min-samples", 10, "minimum samples per benchmark")
	flag.StringVar(&config.prefix, "prefix", "BenchmarkRuntime",
		"required benchmark name prefix; the gate refuses to compare anything outside it")
	flag.Parse()

	result, err := compareFiles(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if result.Status != "pass" {
		os.Exit(1)
	}
}

func compareFiles(config options) (report, error) {
	if config.baselinePath == "" || config.candidatePath == "" {
		return report{}, errors.New("-baseline and -candidate are required")
	}
	if config.alpha <= 0 || config.alpha >= 1 ||
		config.maxRegression < 0 || config.minSamples < 10 {
		return report{}, errors.New("invalid threshold: require 0<alpha<1, max-regression>=0, min-samples>=10")
	}
	baseline, err := parsePath(config.baselinePath, config.prefix)
	if err != nil {
		return report{}, fmt.Errorf("baseline: %w", err)
	}
	candidate, err := parsePath(config.candidatePath, config.prefix)
	if err != nil {
		return report{}, fmt.Errorf("candidate: %w", err)
	}
	if err := sameConfig(baseline.Config, candidate.Config); err != nil {
		return report{}, err
	}
	names, err := sameBenchmarkSet(baseline.Samples, candidate.Samples)
	if err != nil {
		return report{}, err
	}
	result := report{
		SchemaVersion: 1,
		Status:        "pass",
		Alpha:         config.alpha,
		MaxRegression: config.maxRegression,
		MinSamples:    config.minSamples,
		Config:        baseline.Config,
	}
	thresholds := benchmath.DefaultThresholds
	thresholds.CompareAlpha = config.alpha
	for _, name := range names {
		base := baseline.Samples[name]
		next := candidate.Samples[name]
		if len(base.TimeNS) < config.minSamples || len(next.TimeNS) < config.minSamples {
			return report{}, fmt.Errorf(
				"benchmark %q has %d/%d samples, require at least %d",
				name, len(base.TimeNS), len(next.TimeNS), config.minSamples,
			)
		}
		baseSample := benchmath.NewSample(append([]float64(nil), base.TimeNS...), &thresholds)
		nextSample := benchmath.NewSample(append([]float64(nil), next.TimeNS...), &thresholds)
		baseSummary := benchmath.AssumeNothing.Summary(baseSample, 0.95)
		nextSummary := benchmath.AssumeNothing.Summary(nextSample, 0.95)
		comparison := benchmath.AssumeNothing.Compare(baseSample, nextSample)
		item := benchmarkReport{
			Benchmark:          name,
			BaselineSamples:    len(base.TimeNS),
			CandidateSamples:   len(next.TimeNS),
			BaselineMedianNS:   baseSummary.Center,
			CandidateMedianNS:  nextSummary.Center,
			PValue:             comparison.P,
			Significant:        comparison.P < config.alpha,
			BaselineMaxBytes:   maximum(base.Bytes),
			CandidateMaxBytes:  maximum(next.Bytes),
			BaselineMaxAllocs:  maximum(base.Allocs),
			CandidateMaxAllocs: maximum(next.Allocs),
			Status:             "pass",
		}
		if baseSummary.Center == 0 {
			if nextSummary.Center != 0 {
				item.Delta = math.Inf(1)
			}
		} else {
			item.Delta = nextSummary.Center/baseSummary.Center - 1
		}
		if item.CandidateMaxBytes > item.BaselineMaxBytes {
			item.Reasons = append(item.Reasons, "B/op exceeds baseline")
		}
		if item.CandidateMaxAllocs > item.BaselineMaxAllocs {
			item.Reasons = append(item.Reasons, "allocs/op exceeds baseline")
		}
		if item.Significant && item.Delta > config.maxRegression {
			item.Reasons = append(item.Reasons, "statistically significant ns/op regression exceeds threshold")
		}
		item.Warnings = appendWarnings(
			item.Warnings,
			baseSample.Warnings, nextSample.Warnings,
			baseSummary.Warnings, nextSummary.Warnings, comparison.Warnings,
		)
		if len(item.Reasons) != 0 {
			item.Status = "block"
			result.Status = "block"
		}
		result.Comparisons = append(result.Comparisons, item)
	}
	return result, nil
}

func parsePath(path, prefix string) (benchmarkData, error) {
	file, err := os.Open(path)
	if err != nil {
		return benchmarkData{}, err
	}
	defer file.Close()
	return parseBenchmarks(file, prefix)
}

func parseBenchmarks(input io.Reader, prefix string) (benchmarkData, error) {
	result := benchmarkData{
		Config:  make(map[string]string),
		Samples: make(map[string]*metricSamples),
	}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Benchmark") {
			if err := parseBenchmarkLine(result.Samples, line, prefix); err != nil {
				return benchmarkData{}, err
			}
			continue
		}
		if key, value, ok := strings.Cut(line, ":"); ok {
			key = strings.TrimSpace(key)
			switch key {
			case "goos", "goarch", "pkg", "cpu":
				result.Config[key] = strings.TrimSpace(value)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return benchmarkData{}, err
	}
	for _, key := range []string{"goos", "goarch", "pkg", "cpu"} {
		if result.Config[key] == "" {
			return benchmarkData{}, fmt.Errorf("missing benchmark config %q", key)
		}
	}
	if len(result.Samples) == 0 {
		return benchmarkData{}, errors.New("no benchmark samples")
	}
	return result, nil
}

func parseBenchmarkLine(samples map[string]*metricSamples, line, prefix string) error {
	fields := strings.Fields(line)
	if len(fields) < 4 || !strings.HasPrefix(fields[0], prefix) {
		return fmt.Errorf("invalid runtime benchmark line %q", line)
	}
	if _, err := strconv.ParseUint(fields[1], 10, 64); err != nil {
		return fmt.Errorf("invalid iteration count in %q", line)
	}
	metrics := make(map[string]float64)
	for index := 2; index+1 < len(fields); index += 2 {
		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil {
			return fmt.Errorf("invalid metric value in %q", line)
		}
		metrics[fields[index+1]] = value
	}
	for _, unit := range []string{"ns/op", "B/op", "allocs/op"} {
		if _, ok := metrics[unit]; !ok {
			return fmt.Errorf("benchmark %q lacks %s", fields[0], unit)
		}
	}
	sample := samples[fields[0]]
	if sample == nil {
		sample = &metricSamples{}
		samples[fields[0]] = sample
	}
	sample.TimeNS = append(sample.TimeNS, metrics["ns/op"])
	sample.Bytes = append(sample.Bytes, metrics["B/op"])
	sample.Allocs = append(sample.Allocs, metrics["allocs/op"])
	return nil
}

func sameConfig(left, right map[string]string) error {
	for _, key := range []string{"goos", "goarch", "pkg", "cpu"} {
		if left[key] != right[key] {
			return fmt.Errorf("benchmark config %s differs: %q vs %q", key, left[key], right[key])
		}
	}
	return nil
}

func sameBenchmarkSet(
	left, right map[string]*metricSamples,
) ([]string, error) {
	if len(left) != len(right) {
		return nil, fmt.Errorf("benchmark set size differs: %d vs %d", len(left), len(right))
	}
	names := make([]string, 0, len(left))
	for name := range left {
		if right[name] == nil {
			return nil, fmt.Errorf("candidate lacks benchmark %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func maximum(values []float64) float64 {
	var result float64
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

func appendWarnings(target []string, groups ...[]error) []string {
	for _, group := range groups {
		for _, warning := range group {
			target = append(target, warning.Error())
		}
	}
	return target
}
