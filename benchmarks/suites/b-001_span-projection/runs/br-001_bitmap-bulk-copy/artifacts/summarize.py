#!/usr/bin/env python3
from pathlib import Path
import json, math, re, statistics
art = Path(__file__).resolve().parent
samples = {}
for path in sorted(art.glob('repeat-*.txt')):
    for line in path.read_text().splitlines():
        match = re.match(r'BenchmarkProjectSpan(Baseline|Candidate)/(\S+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op', line)
        if match:
            variant, case, ns, size, allocs = match.groups()
            samples.setdefault(case, {}).setdefault(variant, []).append((float(ns), int(size), int(allocs)))
summary = {}
for case, variants in samples.items():
    row = {}
    for variant, values in variants.items():
        ns = [v[0] for v in values]
        median = statistics.median(ns)
        row[variant] = dict(n=len(values), median_ns=median,
            mad_ns=statistics.median(abs(v-median) for v in ns),
            bytes=statistics.median(v[1] for v in values),
            allocs=statistics.median(v[2] for v in values))
    control, candidate = row['Baseline'], row['Candidate']
    row['ratio'] = candidate['median_ns']/control['median_ns']
    row['noise_band'] = 3*(control['mad_ns']/control['median_ns']+candidate['mad_ns']/candidate['median_ns'])
    row['threshold'] = .70 if case.startswith('Unchanged') else 1.10
    row['latency_pass'] = row['ratio'] <= row['threshold']
    row['allocation_pass'] = candidate['bytes'] <= control['bytes'] and candidate['allocs'] <= control['allocs']
    row['margin_pass'] = row['threshold']-row['ratio'] > row['noise_band']
    assert control['n'] == candidate['n'] == 8
    summary[case] = row
ratios = [row['ratio'] for case,row in summary.items() if not case.startswith('Unchanged')]
geo = math.exp(sum(map(math.log,ratios))/len(ratios))
result = dict(cases=summary, slow_geometric_mean=geo)
(art/'summary.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
