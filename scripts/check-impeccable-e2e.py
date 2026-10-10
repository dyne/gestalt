#!/usr/bin/env python3
"""Classify existing upstream JUnit results; never skip or alter a test."""
import json
import pathlib
import sys
import xml.etree.ElementTree as ET

report = ET.parse(sys.argv[1]).getroot()
failures = []
tests = list(report.iter('testcase'))
known = {
    ('live-e2e · astro-vite7 (plain-css)', 'drives the full click → Go → cycle → accept cycle'): ('500', 'astro&type=style&index=1'),
    ('live-e2e · vite8-react-plain (plain-css)', 'Edit copy → Save → Apply/commit: React headless manual Apply hard batch'): ('visible text span.secondary-action', 'Secondary duplicate action'),
}
def visit(element, ancestry=()):
    if element.tag == 'testsuite':
        ancestry += (element.get('name', ''),)
    if element.tag == 'testcase':
        errors = list(element.findall('failure')) + list(element.findall('error'))
        if errors:
            text = '\n'.join((e.get('message', '') + ''.join(e.itertext())) for e in errors)
            matches = [(suite, element.get('name')) for suite in ancestry if (suite, element.get('name')) in known]
            if len(matches) != 1 or not all(fragment in text for fragment in known[matches[0]]):
                raise SystemExit('New/different upstream E2E failure: ' + element.get('name', 'unknown'))
            failures.append(matches[0])
    for child in element:
        visit(child, ancestry)
visit(report)
if len(tests) != 47 or len(failures) != len(set(failures)):
    raise SystemExit('Incomplete/duplicate upstream E2E result')
skipped = [t for t in tests if t.find('skipped') is not None]
if len(skipped) != 1 or skipped[0].get('name') != 'recovers when the preflight reload makes the browser miss the done broadcast':
    raise SystemExit('New/different upstream E2E skip')
if int(sys.argv[3]) != (1 if failures else 0):
    raise SystemExit('E2E process exit differs from complete test results')
result = {'tests':len(tests), 'failures':len(failures), 'knownBaselineLimitations':failures,
          'skipped':len(skipped),
          'state':'known-upstream-limitations' if failures else 'passed'}
pathlib.Path(sys.argv[2]).write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result))
