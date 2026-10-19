#!/usr/bin/env python3
"""Run the predeclared paired projection microbenchmark from repository root."""
import os
from pathlib import Path
import subprocess
import sys

artifacts = Path(sys.argv[1]).resolve()
artifacts.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, GOMAXPROCS="1")
def run(args, target):
    with (artifacts / target).open("w") as output:
        subprocess.run(args, env=env, stdout=output, stderr=subprocess.STDOUT, check=True)

run(["go", "version"], "go-version.txt")
run(["go", "env", "GOOS", "GOARCH", "GOVERSION", "GOMOD"], "go-env.txt")
run(["uname", "-a"], "os.txt")
run(["go", "test", "./semconvweave"], "correctness.txt")
binary = artifacts / "projection.test"
run(["go", "test", "-c", "-o", str(binary), "./semconvweave"], "build.txt")
base = [str(binary), "-test.run=^$", "-test.benchmem"]
with (artifacts / "warmup.txt").open("w") as output:
    subprocess.run(base + ["-test.bench=^BenchmarkProjectSpan", "-test.benchtime=100ms"], env=env, stdout=output, stderr=subprocess.STDOUT, check=True)
for repeat in range(8):
    variants = ["Baseline", "Candidate"] if repeat % 2 == 0 else ["Candidate", "Baseline"]
    for variant in variants:
        run(base + [f"-test.bench=^BenchmarkProjectSpan{variant}$", "-test.benchtime=200ms"], f"repeat-{repeat+1:02d}-{variant}.txt")
    print(f"completed paired repetition {repeat+1}/8", flush=True)
run(["go", "test", "-run", "^$", "-gcflags=github.com/XiaoWeiKIN/otel-semconv-normalizer/semconvweave=-m=2", "./semconvweave"], "compiler.txt")
for variant in ["Baseline", "Candidate"]:
    profile = artifacts / f"cpu-{variant}.pprof"
    run(base + [f"-test.bench=^BenchmarkProjectSpan{variant}$/^Unchanged32$", "-test.benchtime=1s", f"-test.cpuprofile={profile}"], f"profile-{variant}.txt")
    run(["go", "tool", "pprof", "-top", str(binary), str(profile)], f"cpu-{variant}-top.txt")
import hashlib
(artifacts / "binary-sha256.txt").write_text(hashlib.sha256(binary.read_bytes()).hexdigest()+"\n")
binary.unlink()
