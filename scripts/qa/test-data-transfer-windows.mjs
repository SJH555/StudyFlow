// Windows 桌面验收使用隔离二进制，只有原生文件选择结果替换为夹具路径。
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { execFileSync, spawn } from "node:child_process";
import { connect, pause, ps, until, windowsPath } from "./test-startup-memory-windows.mjs";

const root = process.argv[2];
const exportFile = path.join(root, "portable", "学习备份.studyflow");
const port = "http://127.0.0.1:19349/json/list";
const targets = () => fetch(port, { signal: AbortSignal.timeout(500) })
  .then((response) => response.json()).catch(() => []);
const sql = (variant, statement) => execFileSync("python3", [
  "-c",
  "import sqlite3,sys; print(sqlite3.connect(sys.argv[1]).execute(sys.argv[2]).fetchone()[0])",
  path.join(root, variant, "data", "study.db"), statement,
], { encoding: "utf8" }).trim();

async function open(variant) {
  assert.equal((await targets()).length, 0, "调试端口不应占用");
  spawn(ps, [
    "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
    windowsPath(path.join(root, "launch.ps1")), "-Variant", variant,
  ], { stdio: "ignore" }).unref();
  return connect();
}

async function quit(client) {
  await client.evaluate(`void fetch('/wails/runtime',{
    method:'POST',
    headers:{'Content-Type':'application/json','x-wails-client-id':'transfer-qa'},
    body:JSON.stringify({object:2,method:2})})`).catch(() => {});
  client.ws.close();
  await until(async () => !(await targets()).length, "窗口退出");
}

async function settings(client) {
  await until(() => client.evaluate(
    "!!document.querySelector('[aria-label=设置]') && !document.querySelector('.loading')",
  ), "工作区就绪");
  await client.evaluate("document.querySelector('[aria-label=设置]').click()");
  await until(() => client.evaluate("!!document.querySelector('.section-subnav')"), "设置页就绪");
}

async function storage(client) {
  await client.evaluate(`Array.from(document.querySelectorAll('.section-subnav .v-list-item'))
    .find((item) => item.textContent.includes('数据与备份')).click()`);
  await until(() => client.evaluate("!!document.querySelector('.backup-files')"), "备份入口就绪");
}

async function click(client, selector, label) {
  await client.evaluate(`Array.from(document.querySelectorAll(${JSON.stringify(selector)}))
    .find((item) => item.textContent.trim() === ${JSON.stringify(label)}).click()`);
}

async function screenshot(client, name) {
  const shot = await client.send("Page.captureScreenshot", { format: "png" });
  await fs.writeFile(path.join(root, name), Buffer.from(shot.data, "base64"));
}

let client = await open("before");
try {
  await settings(client);
  const leftGeneral = await client.evaluate(
    "document.querySelector('.section-column .section-setting-copy').getBoundingClientRect().left",
  );
  await screenshot(client, "general.png");
  for (const [label, selector] of [["日志", ".settings-log-location"], ["重置数据", ".settings-reset-note"]]) {
    await click(client, ".section-subnav .v-list-item", label);
    const left = await until(() => client.evaluate(
      `document.querySelector(${JSON.stringify(selector)})?.getBoundingClientRect().left`,
    ), label + "面板");
    assert.ok(Math.abs(left - leftGeneral) < 1, label + "与常规页的左边距不同");
  }
  await storage(client);
  const layout = await client.evaluate(`(() => {
    const left = selector => document.querySelector(selector).getBoundingClientRect().left;
    const theme = getComputedStyle(document.querySelector('.backup-file-row'));
    const buttons = [...document.querySelectorAll('.backup-file-actions button')];
    return {
      left: ['.settings-block-heading', '.settings-backup-row .section-setting-copy', '.backup-files h2'].map(left),
      background: theme.backgroundColor,
      heading: document.querySelector('.backup-files h2').textContent.trim(),
      tonal: buttons.every(button => button.classList.contains('text-primary') && button.classList.contains('v-btn--variant-tonal')),
    };
  })()`);
  assert.ok(layout.left.every((left) => Math.abs(left - leftGeneral) < 1), JSON.stringify({ leftGeneral, layout }));
  assert.ok(layout.tonal);
  assert.equal(layout.heading, "数据迁移");
  assert.equal(layout.background, "rgba(0, 0, 0, 0)");
  await screenshot(client, "storage.png");
  await click(client, ".backup-file-actions button", "导出");
  await until(() => fs.stat(exportFile).then((stat) => stat.size > 0), "导出到指定子目录");
  await until(() => client.evaluate("!document.querySelector('.backup-file-actions button').disabled"), "导出完成");
  console.log("PASS: aligned settings, theme buttons, .studyflow exported", layout);
} finally {
  await quit(client);
}

client = await open("after");
try {
  await settings(client);
  await storage(client);
  await click(client, ".backup-file-actions button", "恢复");
  await until(() => client.evaluate("!!document.querySelector('.restore-preview')"), "恢复预览");
  assert.match(await client.evaluate("document.querySelector('.restore-preview').textContent"), /成功后不保留原数据库/);
  await pause(350);
  await screenshot(client, "restore-preview.png");
  assert.equal(sql("after", "SELECT name FROM subjects"), "目标原库");
  await click(client, ".restore-actions button", "取消");
  await until(() => client.evaluate("!document.querySelector('.v-overlay--active .restore-preview')"), "取消预览");
  assert.equal(sql("after", "SELECT name FROM subjects"), "目标原库");

  await click(client, ".backup-file-actions button", "恢复");
  await until(() => client.evaluate("!!document.querySelector('.v-overlay--active .restore-preview')"), "再次预览");
  await click(client, ".restore-actions button", "替换数据");
  await until(() => client.evaluate("document.querySelector('#restore-title')?.textContent.includes('数据已恢复')"), "替换成功");
  assert.equal(sql("after", "SELECT name FROM subjects"), "迁移测试来源");
  assert.equal(sql("after", "SELECT value FROM settings WHERE key='activityStartTime'"), "10:35");
  const entries = await fs.readdir(path.join(root, "after", "data"));
  assert.ok(entries.every((name) => !name.startsWith(".studyflow-rollback-")));
  assert.ok(await fs.stat(exportFile));
  const oldSocket = client.ws.url;
  await click(client, ".restore-actions button", "重新打开");
  client.ws.close();
  await until(async () => (await targets()).some(
    (page) => page.url === "http://wails.localhost/" && page.webSocketDebuggerUrl !== oldSocket,
  ), "程序重开");
  client = await connect();
  await until(() => client.evaluate(
    "document.querySelector('.today-records-panel .record-title b')?.textContent === '迁移测试来源'",
  ), "重新打开后，新库记录可见");
  await pause(50);
  console.log("PASS: preview cancellation preserved original and restore reopened with one live copy");
} finally {
  await quit(client);
}
