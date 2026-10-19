// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareFilesPassesNoiseAndBlocksLargeSignificantRegression(t *testing.T) {
	tests := []struct {
		name      string
		base      []float64
		candidate []float64
		want      string
	}{
		{
			name:      "identical",
			base:      values(100, 20, 0),
			candidate: values(100, 20, 0),
			want:      "pass",
		},
		{
			name:      "large regression",
			base:      values(100, 20, 0.1),
			candidate: values(120, 20, 0.1),
			want:      "block",
		},
		{
			name:      "large improvement",
			base:      values(120, 20, 0.1),
			candidate: values(100, 20, 0.1),
			want:      "pass",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := writeBenchmark(t, test.base, 0, 0, "Apple M4")
			candidate := writeBenchmark(t, test.candidate, 0, 0, "Apple M4")
			got, err := compareFiles(options{
				baselinePath:  base,
				candidatePath: candidate,
				alpha:         0.05,
				maxRegression: 0.05,
				minSamples:    10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != test.want {
				t.Fatalf("status = %q, want %q: %+v", got.Status, test.want, got.Comparisons)
			}
		})
	}
}

func TestCompareFilesAllocationIncreaseAlwaysBlocks(t *testing.T) {
	base := writeBenchmark(t, values(100, 20, 0), 0, 0, "Apple M4")
	candidate := writeBenchmark(t, values(100, 20, 0), 8, 1, "Apple M4")
	got, err := compareFiles(options{
		baselinePath:  base,
		candidatePath: candidate,
		alpha:         0.05,
		maxRegression: 0.05,
		minSamples:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "block" ||
		len(got.Comparisons) != 1 ||
		len(got.Comparisons[0].Reasons) != 2 {
		t.Fatalf("allocation regression report = %+v", got)
	}
}

func TestCompareFilesRejectsInsufficientSamplesAndConfigMismatch(t *testing.T) {
	base := writeBenchmark(t, values(100, 9, 0), 0, 0, "Apple M4")
	candidate := writeBenchmark(t, values(100, 9, 0), 0, 0, "Apple M4")
	_, err := compareFiles(options{
		baselinePath:  base,
		candidatePath: candidate,
		alpha:         0.05,
		maxRegression: 0.05,
		minSamples:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "at least 10") {
		t.Fatalf("insufficient sample error = %v", err)
	}

	candidate = writeBenchmark(t, values(100, 20, 0), 0, 0, "Other CPU")
	base = writeBenchmark(t, values(100, 20, 0), 0, 0, "Apple M4")
	_, err = compareFiles(options{
		baselinePath:  base,
		candidatePath: candidate,
		alpha:         0.05,
		maxRegression: 0.05,
		minSamples:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "cpu differs") {
		t.Fatalf("config mismatch error = %v", err)
	}
}

func TestParseBenchmarksRejectsMissingMetrics(t *testing.T) {
	input := strings.NewReader(`goos: darwin
goarch: arm64
pkg: example.test
cpu: Apple M4
BenchmarkRuntimeResolve/canonical-10 100 4.0 ns/op
`)
	_, err := parseBenchmarks(input, "BenchmarkRuntime")
	if err == nil || !strings.Contains(err.Error(), "lacks B/op") {
		t.Fatalf("parse error = %v", err)
	}
}

func values(center float64, count int, step float64) []float64 {
	result := make([]float64, count)
	for index := range result {
		result[index] = center + float64(index)*step
	}
	return result
}

func writeBenchmark(
	t *testing.T,
	times []float64,
	bytes, allocs float64,
	cpu string,
) string {
	t.Helper()
	var output strings.Builder
	fmt.Fprintf(&output, "goos: darwin\ngoarch: arm64\npkg: example.test\ncpu: %s\n", cpu)
	for _, value := range times {
		fmt.Fprintf(
			&output,
			"BenchmarkRuntimeResolve/canonical-10 1000000 %.3f ns/op %.0f B/op %.0f allocs/op\n",
			value, bytes, allocs,
		)
	}
	path := filepath.Join(t.TempDir(), "benchmark.txt")
	if err := os.WriteFile(path, []byte(output.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
