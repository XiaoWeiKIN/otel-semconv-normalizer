#!/usr/bin/env python3
"""Summarize every BS-002 timing sample, without exclusions."""
import json
import math
from pathlib import Path
import re
import statistics
import sys

art = Path(sys.argv[1])
samples = {}
for path in sorted(art.glob('repeat-*.txt')):
    for line in path.read_text().splitlines():
        match = re.match(r'BenchmarkSelectiveProjection(Prior|Candidate)/(\S+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op', line)
        if match:
            variant, case, ns, size, allocs = match.groups()
            samples.setdefault(case,{}).setdefault(variant,[]).append((float(ns),int(size),int(allocs)))
assert len(samples) == 21, f'incomplete case inventory: {len(samples)}'
summary = {}
for case, variants in samples.items():
    row = {}
    for variant, values in variants.items():
        assert len(values) == 8, (case,variant,len(values))
        ns = [v[0] for v in values]
        median = statistics.median(ns)
        row[variant] = dict(n=len(values), median_ns=median, mad_ns=statistics.median(abs(v-median) for v in ns), bytes=statistics.median(v[1] for v in values), allocs=statistics.median(v[2] for v in values))
    control, candidate = row['Prior'], row['Candidate']
    row['ratio'] = candidate['median_ns']/control['median_ns']
    row['noise_band'] = 3*(control['mad_ns']/control['median_ns']+candidate['mad_ns']/candidate['median_ns'])
    row['threshold'] = .75 if case.startswith('Sparse') else .80 if case == 'CanonicalHTTP8' else 1.10
    row['latency_pass'] = row['ratio'] <= row['threshold']
    row['allocation_pass'] = candidate['allocs'] <= control['allocs'] and candidate['bytes'] <= control['bytes']*(1.15 if case.startswith('Sparse') else 1)
    row['margin_pass'] = abs(row['threshold']-row['ratio']) > row['noise_band']
    summary[case] = row
protocols = [summary[c] for c in ['HTTP','DB','RPC','MissingContext','Invalid']]
geo = math.exp(statistics.mean(math.log(row['ratio']) for row in protocols))
geo_band = statistics.mean(row['noise_band'] for row in protocols)
if not all(row['allocation_pass'] for row in summary.values()):
    outcome = 'failed'
elif not all(row['margin_pass'] for row in summary.values()) or abs(.95-geo) <= geo_band:
    outcome = 'inconclusive'
elif geo > .95 or not all(row['latency_pass'] for row in summary.values()):
    outcome = 'failed'
else:
    outcome = 'passed'
result = dict(cases=summary, protocol_geometric_mean=geo, protocol_noise_band=geo_band, outcome=outcome)
(art/'summary.json').write_text(json.dumps(result,indent=2)+'\n')
for case,row in summary.items():
    print(f"{case}: {row['Prior']['median_ns']:.2f} -> {row['Candidate']['median_ns']:.2f} ns/op; ratio={row['ratio']:.3f}; B/op={row['Prior']['bytes']}->{row['Candidate']['bytes']}; allocs={row['Prior']['allocs']}->{row['Candidate']['allocs']}")
print(f'protocol geometric mean={geo:.4f}; outcome={outcome}')
