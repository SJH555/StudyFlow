// 在真实 Windows WebView 中隔离图表和外壳的图形资源占用。
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { spawn } from "node:child_process";
import {
  connect,
  until,
  pause,
  windowsPath,
  powershell,
  ps,
} from "./test-startup-memory-windows.mjs";

const root = process.argv[2];
const out = process.argv[3];
const idleMs = Number(process.env.GRAPHICS_IDLE_MS || 5000);
const runs = Number(process.env.GRAPHICS_RUNS || 3);
const modes = JSON.parse(
  await fs.readFile(path.join(root, "graphics.json"), "utf8"),
).modes;
const orders = [
  modes,
  [...modes].reverse(),
  ["none", "svg", "shell", "canvas"],
];
await fs.mkdir(out, { recursive: true });
const results = [];
let expectedViewport;
let expectedData;

async function snapshot(client) {
  const metrics = await client.send("Performance.getMetrics");
  const surface = await client.evaluate(`(() => {
    const canvases=[...document.querySelectorAll('.chart canvas,.project-chart canvas')];
    return {
      width:innerWidth,height:innerHeight,dpr:devicePixelRatio,
      chartCanvasCount:canvases.length,
      canvasRgbaBytes:canvases.reduce((sum,c)=>sum+c.width*c.height*4,0),
      chartSvgCount:document.querySelectorAll('.chart svg,.project-chart svg').length,
      chartSvgNodes:document.querySelectorAll('.chart svg *,.project-chart svg *').length,
    };
  })()`);
  const native = JSON.parse(
    powershell("memory.ps1", "-RootPid", String(client.pid)),
  );
  const measured = Object.fromEntries(
    metrics.metrics
      .filter((item) =>
        ["JSHeapUsedSize", "JSHeapTotalSize", "Nodes", "Documents"].includes(
          item.name,
        ),
      )
      .map((item) => [item.name, item.value]),
  );
  const viewport = {
    width: surface.width,
    height: surface.height,
    dpr: surface.dpr,
  };
  if (!expectedViewport) expectedViewport = viewport;
  assert.deepEqual(viewport, expectedViewport, "窗口和缩放必须保持一致");
  return { native, measured, surface };
}

async function settledSnapshot(client) {
  await pause(idleMs);
  return snapshot(client);
}

async function openStats(client, mode) {
  const started = performance.now();
  await client.evaluate(`(() => {
    const stats=window.qaPinia._s.get('stats');
    stats.statsPeriod='month';stats.overviewPeriod='month';
    document.querySelector('[aria-label="学习统计"]').click();
  })()`);
  await until(
    () =>
      client.evaluate(`(() => {
    const stats=window.qaPinia._s.get('stats');
    return stats.statsReady && !stats.statsLoading;
  })()`),
    "统计页就绪",
  );
  if (mode !== "none") {
    assert.equal(
      await client.evaluate(`(() => {
      const charts=window.qaPinia._s.get('stats').charts;
      return charts.getChart()?.getDom()===document.querySelector('.chart') &&
        charts.getProjectChart()?.getDom()===document.querySelector('.project-chart');
    })()`),
      true,
      "图表必须绑定当前容器",
    );
  }
  return performance.now() - started;
}

async function dataSignature(client) {
  return client.evaluate(`(() => {
    const stats=window.qaPinia._s.get('stats');
    return {
      count:stats.overviewSessionCount,seconds:stats.overviewActualSeconds,
      days:Object.keys(stats.trendSessions).sort(),
      actual:stats.periodDates.map(date=>stats.dailyActualSeconds(date)),
    };
  })()`);
}

for (let run = 0; run < runs; run++) {
  for (const mode of orders[run % orders.length]) {
    const busy = await fetch("http://127.0.0.1:19349/json/list", {
      signal: AbortSignal.timeout(500),
    })
      .then((response) => response.ok)
      .catch(() => false);
    assert.equal(busy, false, "调试端口占用，拒绝串用测试实例");
    const pidFile = path.join(root, mode, "pid.txt");
    await fs.rm(pidFile, { force: true });
    spawn(
      ps,
      [
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        windowsPath(path.join(root, "launch.ps1")),
        "-Variant",
        mode,
      ],
      { stdio: "ignore" },
    ).unref();
    const pid = await until(
      async () => Number(await fs.readFile(pidFile, "utf8")),
      "测试进程",
    );
    const client = await connect();
    client.pid = pid;
    const errors = [];
    client.ws.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.method === "Runtime.exceptionThrown")
        errors.push(message.params);
    });
    const record = { run, mode };
    try {
      await client.send("Runtime.enable");
      await client.send("Performance.enable");
      record.agent = await client.evaluate("navigator.userAgent");
      if (mode === "shell") {
        await until(
          () => client.evaluate(`!!document.querySelector('[data-qa-shell]')`),
          "空外壳",
        );
        record.shell = await settledSnapshot(client);
      } else {
        await until(
          () =>
            client.evaluate(
              `!!document.querySelector('[aria-label="学习统计"]') && !document.querySelector('.loading')`,
            ),
          "首页",
        );
        await client.evaluate(`void (window.qaPinia=Object.getOwnPropertySymbols(document.querySelector('#app').__vue_app__._context.provides)
          .map(key=>document.querySelector('#app').__vue_app__._context.provides[key]).find(value=>value?._s instanceof Map))`);
        record.home = await settledSnapshot(client);
        record.entryMs = await openStats(client, mode);
        const signature = await dataSignature(client);
        if (!expectedData) expectedData = signature;
        assert.deepEqual(signature, expectedData, "渲染方式不应改变统计值");
        record.data = signature;
        record.stats = await settledSnapshot(client);
        record.refreshes = [];
        for (let index = 0; index < 3; index++) {
          const elapsed = await client.evaluate(`(async()=>{
            const started=performance.now();
            await window.qaPinia._s.get('stats').refreshStatsPage();
            return performance.now()-started;
          })()`);
          record.refreshes.push(elapsed);
        }
        record.afterRefreshImmediate = await snapshot(client);
        record.afterRefresh = await settledSnapshot(client);
        await client.evaluate(
          `document.querySelector('[aria-label="今日计划"]').click()`,
        );
        await until(
          () =>
            client.evaluate(
              `window.qaPinia._s.get('app').page==='today' && !document.querySelector('.stats-main')`,
            ),
          "离开统计页",
        );
        assert.equal(
          await client.evaluate(`window.qaPinia._s.get('stats').charts===null`),
          true,
        );
        record.leave = await settledSnapshot(client);
        await client.send("HeapProfiler.collectGarbage");
        record.leaveAfterGC = await snapshot(client);
        // 截图和导出检查放在内存采样后，避免图像捕获污染前面的稳定值。
        await openStats(client, mode);
        await pause(1500);
        if (mode !== "none") {
          record.exportPrefix = await client.evaluate(
            `window.qaPinia._s.get('stats').charts.getChart().getDataURL({type:'png',pixelRatio:2}).slice(0,48)`,
          );
          record.series = await client.evaluate(
            `window.qaPinia._s.get('stats').charts.getChart().getOption().series.map(s=>({name:s.name,data:s.data}))`,
          );
        }
      }
      await fs.writeFile(
        path.join(out, `${mode}-${run}.png`),
        Buffer.from(
          (await client.send("Page.captureScreenshot", { format: "png" })).data,
          "base64",
        ),
      );
      assert.equal(errors.length, 0, "不应发生前端异常");
      results.push(record);
      await fs.writeFile(
        path.join(out, "results.json"),
        JSON.stringify(results, null, 2),
      );
      const sampled = record.stats || record.shell;
      console.log(
        "PASS",
        run,
        mode,
        "private MiB",
        Math.round(sampled.native.privateBytes / 1048576),
      );
    } finally {
      await client
        .evaluate(
          `void fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':'graphics-qa'},body:JSON.stringify({object:2,method:2})})`,
        )
        .catch(() => {});
      client.ws.close();
      await until(
        async () =>
          !(await fetch("http://127.0.0.1:19349/json/list", {
            signal: AbortSignal.timeout(500),
          })
            .then((r) => r.ok)
            .catch(() => false)),
        "测试窗口退出",
      );
    }
  }
}
