package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/XiaoWeiKIN/otel-semconv-normalizer/tools/semconv-weave-gen/internal/inputlock"
)

func main() {
	root := flag.String("root", "", "path to the pinned fixture root")
	flag.Parse()
	if *root == "" {
		fmt.Fprintln(os.Stderr, "verify-inputs: -root is required")
		os.Exit(2)
	}
	if err := inputlock.Verify(*root); err != nil {
		fmt.Fprintf(os.Stderr, "verify-inputs: %v\n", err)
		os.Exit(1)
	}
}
