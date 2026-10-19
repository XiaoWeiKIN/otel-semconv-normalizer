// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/typedir"
)

func main() {
	fixtureRoot := flag.String("root", "testdata", "checksum-locked build input root")
	weaverRoot := flag.String("weaver-root", "build/weaver", "promoted Weaver evidence root")
	output := flag.String("output", "build/typed-ir.json", "private Typed IR output")
	flag.Parse()

	bundle, err := typedir.Compile(typedir.Config{
		FixtureRoot: *fixtureRoot,
		WeaverRoot:  *weaverRoot,
	})
	if err != nil {
		fail(err)
	}
	data, err := typedir.Encode(bundle)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
