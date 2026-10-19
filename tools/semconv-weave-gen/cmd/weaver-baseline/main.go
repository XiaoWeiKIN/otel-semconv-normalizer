package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/weaverbaseline"
)

func main() {
	inputs := flag.String("inputs", "testdata", "checksum-locked local input root")
	output := flag.String("output", "build/weaver", "promoted Weaver evidence directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "weaver-baseline accepts flags only")
		os.Exit(2)
	}
	if err := weaverbaseline.Run(context.Background(), weaverbaseline.Config{
		Inputs: *inputs,
		Output: *output,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
