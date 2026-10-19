#!/usr/bin/env python3
"""Execute BS-002 from the repository root; --identity prints pre-run digests."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

root = Path.cwd()
scripts = Path(__file__).resolve().parent
subject = sorted([p for p in Path('semconvweave').glob('*.go') if not p.name.endswith('_test.go')] + [Path('go.mod'), Path('go.sum'), Path('semconvweave/source_manifest.json')])
harness = sorted(list(Path('semconvweave').glob('*_test.go')) + list(Path('semconvweave/testdata').rglob('*')) + list(Path('tools/semconv-weave-gen/testdata/audit').glob('*.json')) + list(Path('tools/semconv-weave-gen/testdata/guides').glob('*.md')) + [scripts.relative_to(root)/'run_selective.py', scripts.relative_to(root)/'summarize_selective.py'])
harness = [p for p in harness if p.is_file()]

def inventory(paths):
    return {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}

def identity(entries):
    return hashlib.sha256(json.dumps(entries, sort_keys=True, separators=(',', ':')).encode()).hexdigest()

manifest = {'subject': inventory(subject), 'harness': inventory(harness)}
manifest['subject_revision'] = 'sha256:' + identity(manifest['subject'])
manifest['harness_revision'] = 'sha256:' + identity(manifest['harness'])
if sys.argv[1] == '--identity':
    print(json.dumps(manifest, indent=2))
    sys.exit(0)

art = Path(sys.argv[1]).resolve()
art.mkdir(parents=True, exist_ok=True)
(art/'source-manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
for path in subject+harness:
    dest = art/'snapshot'/path
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(path, dest)
env = dict(os.environ, GOMAXPROCS='1')

def run(args, name):
    with (art/name).open('w') as output:
        subprocess.run(args, env=env, stdout=output, stderr=subprocess.STDOUT, check=True)

run(['go','version'], 'go-version.txt')
run(['go','env','GOOS','GOARCH','GOVERSION','GOMOD'], 'go-env.txt')
run(['uname','-a'], 'os.txt')
run(['sysctl','-n','machdep.cpu.brand_string'], 'cpu.txt')
run(['go','test','./semconvweave'], 'correctness.txt')
binary = art/'projection.test'
run(['go','test','-c','-o',str(binary),'./semconvweave'], 'build.txt')
base = [str(binary), '-test.run=^$', '-test.benchmem']
run(base+['-test.bench=^BenchmarkSelectiveProjection', '-test.benchtime=100ms'], 'warmup.txt')
for repeat in range(8):
    variants = ['Prior','Candidate'] if repeat%2 == 0 else ['Candidate','Prior']
    for variant in variants:
        run(base+[f'-test.bench=^BenchmarkSelectiveProjection{variant}$','-test.benchtime=200ms'], f'repeat-{repeat+1:02d}-{variant}.txt')
    print(f'completed paired repetition {repeat+1}/8', flush=True)
run(['go','test','-run','^$','-gcflags=github.com/XiaoWeiKIN/otel-semconv-normalizer/semconvweave=-m=2','./semconvweave'], 'compiler.txt')
run(['go','tool','objdump','-s','semconvweave.*(ProjectSpan|projectSparseAttributeMap)',str(binary)], 'objdump.txt')
(art/'binary-sha256.txt').write_text(hashlib.sha256(binary.read_bytes()).hexdigest()+'\n')
binary.unlink()
subprocess.run([sys.executable,str(scripts/'summarize_selective.py'),str(art)], check=True)
