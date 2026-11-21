#!/usr/bin/env python3
"""构建四种隔离 Windows 图形实验，结束后恢复正常前端资源。"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
from tempfile import TemporaryDirectory

repo = Path(__file__).resolve().parents[2]
cache = Path.home() / '.cache' / 'studyflow-graphics-qa'
cache.mkdir(parents=True, exist_ok=True)
main = repo / 'desktop/main.go'
source = main.read_text()
patched = source.replace(
    'configDir = filepath.Join(configDir, "StudyTime")',
    'configDir = filepath.Join(installDir, "legacy")',
).replace(
    'application.WindowsOptions{DisableQuitOnLastWindowClosed: true}',
    'application.WindowsOptions{DisableQuitOnLastWindowClosed: true, '
    'AdditionalBrowserArgs: []string{"--remote-debugging-port=19349"}, '
    'WebviewUserDataPath: filepath.Join(baseDir, "webview-profile")}',
)
assert patched != source and '--remote-debugging-port' in patched


def run(*args, **kwargs):
    subprocess.run(args, cwd=repo, check=True, **kwargs)

arch = "amd64"
with TemporaryDirectory(prefix="studyflow-graphics-build-") as tmp:
    staging = Path(tmp)
    patched_main = staging / "main-ui.go"
    patched_main.write_text(patched)
    overlay = staging / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(main): str(patched_main)}}))

    resource = repo / "desktop" / f"wails_windows_{arch}.syso"
    # Go 打包时从源码目录读取 .syso。独占创建，避免覆盖已有的构建资源。
    with resource.open("xb"):
        pass
    try:
        run("wails3", "task", "windows:generate:syso", f"ARCH={arch}")
        try:
            binaries = {}
            for mode in ['shell', 'none', 'canvas', 'svg']:
                env = dict(os.environ, STUDYFLOW_GRAPHICS_QA=mode)
                run('node', 'vue/node_modules/vite/bin/vite.js', 'build',
                    '--config', 'vue/vite.graphics-qa.config.ts', env=env)
                # 只暂存生成产物，desktop/ui 始终由项目同步工具生成。
                staged = repo / 'vue/dist'
                if staged.exists():
                    shutil.rmtree(staged)
                shutil.copytree(cache / mode / 'dist', staged)
                run('go', 'run', './tools/syncassets')
                binary = cache / mode / 'StudyFlow.exe'
                env = dict(os.environ, GOOS='windows', GOARCH=arch, CGO_ENABLED='0')
                run('go', 'build', '-overlay', str(overlay), '-tags', 'production',
                    '-trimpath', '-ldflags=-w -s -H windowsgui', '-o', str(binary), './desktop', env=env)
                binaries[mode] = hashlib.sha256(binary.read_bytes()).hexdigest()
            (cache / 'binaries.json').write_text(json.dumps(binaries, indent=2))
        finally:
            run('npm', 'run', 'build', '--prefix', 'vue')
            run('go', 'run', './tools/syncassets')
    finally:
        resource.unlink(missing_ok=True)
print(cache)
