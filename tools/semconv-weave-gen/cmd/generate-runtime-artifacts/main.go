// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/emitter"
)

func main() {
	fixtureRoot := flag.String("root", "testdata", "checksum-locked build input root")
	weaverRoot := flag.String("weaver-root", "build/weaver", "promoted Weaver evidence root")
	providerRoot := flag.String("provider-root", "../..", "provider repository root")
	output := flag.String("output", "", "fresh staging output directory (use an _-prefixed directory inside this Go module)")
	check := flag.Bool("check", false, "compare fresh emission with checked provider artifacts")
	flag.Parse()

	if *check == (*output != "") {
		fail(fmt.Errorf("choose exactly one of -check or -output"))
	}
	artifacts, err := emitter.Generate(*fixtureRoot, *weaverRoot, *providerRoot)
	if err != nil {
		fail(err)
	}
	if *check {
		if err := emitter.CheckArtifacts(*providerRoot, artifacts); err != nil {
			fail(err)
		}
		return
	}
	if err := emitter.WriteFresh(*output, artifacts); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
