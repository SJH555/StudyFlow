#!/usr/bin/env python3
"""准备空外壳、无图表、Canvas、SVG 四种同数据的隔离夹具。"""
import json
from pathlib import Path
import shutil
import subprocess
import sys

repo = Path(__file__).resolve().parents[2]
cache = Path.home() / '.cache' / 'studyflow-graphics-qa'
result = subprocess.check_output([
    sys.executable, str(repo / 'scripts/qa/prepare-startup-memory.py'),
    '--before-binary', str(cache / 'canvas/StudyFlow.exe'),
    '--after-binary', str(cache / 'svg/StudyFlow.exe'),
    '--record-mode', 'mixed', '--history-days', '365',
], text=True)
root = Path(result.strip())
assert (root / '.studyflow-qa').read_text() == 'startup memory fixtures\n'
(root / 'before').rename(root / 'canvas')
(root / 'after').rename(root / 'svg')
for mode in ['none', 'shell']:
    target = root / mode
    target.mkdir()
    shutil.copytree(root / 'canvas/data', target / 'data')
    shutil.copyfile(cache / mode / 'StudyFlow.exe', target / 'StudyFlow.exe')
(root / 'graphics.json').write_text(json.dumps({
    'modes': ['shell', 'none', 'canvas', 'svg'],
    'records': 8760,
    'recordMode': 'mixed',
    'binaries': json.loads((cache / 'binaries.json').read_text()),
}, indent=2))
print(root)
