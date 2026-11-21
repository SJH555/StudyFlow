// 使用真实 Windows WebView 与独立测试数据比较启动、图表刷新和内存保留。
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import { execFileSync, spawn } from "node:child_process";
import { pathToFileURL } from "node:url";
const root = process.argv[2];
const out = process.argv[3];
const timeoutMs = Number(process.env.QA_TIMEOUT_MS || 15000);
assert.equal(
  await fs.readFile(path.join(root, ".studyflow-qa"), "utf8"),
  "startup memory fixtures\n",
);
const ps = "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe";
const windowsPath = (file) =>
  execFileSync("wslpath", ["-w", file], { encoding: "utf8" }).trim();
const powershell = (file, ...args) =>
  execFileSync(
    ps,
    [
      "-NoProfile",
      "-ExecutionPolicy",
      "Bypass",
      "-File",
      windowsPath(path.join(root, file)),
      ...args,
    ],
    { encoding: "utf8" },
  ).trim();
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(work, label) {
  for (let attempt = 0; attempt < Math.ceil(timeoutMs / 50); attempt++) {
    try {
      const value = await work();
      if (value) return value;
    } catch {}
    await pause(50);
  }
  throw new Error("等待超时：" + label);
}
async function connect() {
  const page = await until(
    async () =>
      (await (await fetch("http://127.0.0.1:19349/json/list")).json()).find(
        (item) => item.url === "http://wails.localhost/",
      ),
    "WebView",
  );
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
  });
  let id = 0;
  const pending = new Map();
  ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    const callback = pending.get(message.id);
    if (callback) {
      pending.delete(message.id);
      message.error
        ? callback.reject(Error(JSON.stringify(message.error)))
        : callback.resolve(message.result);
    }
  };
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      pending.set(++id, { resolve, reject });
      ws.send(JSON.stringify({ id, method, params }));
    });
  const evaluate = async (expression) => {
    const result = await send("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    });
    if (result.exceptionDetails)
      throw Error(JSON.stringify(result.exceptionDetails));
    return result.result.value;
  };
  return { ws, send, evaluate };
}
const results = [];
const comparisonSnapshots = new Map();
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  for (let run = 0; run < Number(process.env.QA_RUNS || 3); run++) {
    for (const variant of (process.env.QA_VARIANTS || "before,after").split(
      ",",
    )) {
      const debuggerBusy = await fetch("http://127.0.0.1:19349/json/list", {
        signal: AbortSignal.timeout(500),
      })
        .then((response) => response.ok)
        .catch(() => false);
      assert.equal(
        debuggerBusy,
        false,
        "19349 调试端口已占用，拒绝连接其他测试实例",
      );
      const started = performance.now();
      const pidFile = path.join(root, variant, "pid.txt");
      await fs.rm(pidFile, { force: true });
      const launcher = spawn(
        ps,
        [
          "-NoProfile",
          "-ExecutionPolicy",
          "Bypass",
          "-File",
          windowsPath(path.join(root, "launch.ps1")),
          "-Variant",
          variant,
        ],
        { stdio: "ignore" },
      );
      launcher.unref();
      const pid = await until(
        async () => Number(await fs.readFile(pidFile, "utf8")),
        "进程号",
      );
      const client = await connect();
      try {
        await client.send("Performance.enable");
        await until(
          () =>
            client.evaluate(
              `!!document.querySelector('[aria-label="学习统计"]') && !document.querySelector('.loading')`,
            ),
          "首页",
        );
        const launchToHomeMs = performance.now() - started;
        await client.evaluate(
          `window.qaPinia = Object.getOwnPropertySymbols(document.querySelector('#app').__vue_app__._context.provides).map(k => document.querySelector('#app').__vue_app__._context.provides[k]).find(v => v?._s instanceof Map); if (!window.qaPinia) throw Error('Pinia 未找到')`,
        );
        if (process.env.QA_QUERY_PROFILE === "1") {
          await client.evaluate(`(() => {
        const parse=JSON.parse;
        window.qaQueryParse={count:0,totalMs:0,maxMs:0};
        JSON.parse=function(text,...args) {
          const started=performance.now();
          const result=parse.call(JSON,text,...args);
          const elapsed=performance.now()-started;
          if(typeof text==='string' && text.includes('"startAt":')) {
            window.qaQueryParse.count++;
            window.qaQueryParse.totalMs+=elapsed;
            window.qaQueryParse.maxMs=Math.max(window.qaQueryParse.maxMs,elapsed);
          }
          return result;
        };
      })()`);
        }
        const memory = async () => {
          await client.send("HeapProfiler.collectGarbage");
          const metrics = await client.send("Performance.getMetrics");
          return {
            ...JSON.parse(powershell("memory.ps1", "-RootPid", String(pid))),
            metrics: Object.fromEntries(
              metrics.metrics
                .filter((item) =>
                  ["JSHeapUsedSize", "Nodes", "Documents"].includes(item.name),
                )
                .map((item) => [item.name, item.value]),
            ),
          };
        };
        const homeMemory = await memory();
        const switches = [];
        const refreshes = [];
        let statsMemory;
        for (
          let cycle = 0;
          cycle < Number(process.env.QA_CYCLES || 3);
          cycle++
        ) {
          const switching = performance.now();
          await client.evaluate(
            `document.querySelector('[aria-label="学习统计"]').click()`,
          );
          await until(
            () =>
              client.evaluate(
                `(() => { const s=window.qaPinia._s.get('stats'); return s?.charts?.getChart() && !s.statsLoading; })()`,
              ),
            "统计图",
          );
          switches.push(performance.now() - switching);
          const refreshed = await client.evaluate(`(async () => {
        const s=window.qaPinia._s.get('stats');
        s.statsPeriod='month'; s.overviewPeriod='month';
        // 等待周月切换真正结束，避免把旧请求排队时间算作本次刷新耗时。
        const settledAt=performance.now();
        while (Object.keys(s.trendSessions).length!==s.periodDates.length ||
          Object.keys(s.comparisonSessions).length!==s.comparisonDates.length ||
          s.charts.getChart().getOption().xAxis[0].data.length!==s.periodDates.length) {
          if(performance.now()-settledAt>${timeoutMs}) throw Error('周月切换未完成');
          await new Promise(r=>setTimeout(r,25));
        }
        await new Promise(r=>setTimeout(r,50));
        const old=s.charts.getChart();
        if(window.qaQueryParse) window.qaQueryParse={count:0,totalMs:0,maxMs:0};
        const started=performance.now(); await s.refreshStatsPage();
        return {ms:performance.now()-started,same:old===s.charts.getChart(),image:s.charts.getChart().getDataURL({type:'png'}).slice(0,22),series:s.charts.getChart().getOption().series.length,timings:Object.fromEntries(performance.getEntriesByType('measure').filter(m=>m.name.startsWith('studyflow:stats:')).map(m=>[m.name,m.duration]))};
      })()`);
          const correctness = await client.evaluate(`(async () => {
        const stats=window.qaPinia._s.get('stats');
        const normalize=groups=>Object.keys(groups).sort().map(day=>[day,groups[day]]);
        const records=JSON.stringify([normalize(stats.trendSessions),normalize(stats.comparisonSessions)]);
        const digest=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(records));
        return {
          recordHash:Array.from(new Uint8Array(digest)).map(value=>value.toString(16).padStart(2,'0')).join(''),
          overviewSeconds:stats.overviewActualSeconds,
          sessionCount:stats.overviewSessionCount,
          trend:stats.charts.getChart().getOption().series.map(series=>({name:series.name,data:series.data})),
          comparison:stats.charts.getProjectChart()?.getOption().series.map(series=>({name:series.name,data:series.data})),
        };
      })()`);
          if (variant === "before")
            comparisonSnapshots.set(`${run}:${cycle}`, correctness);
          else if (comparisonSnapshots.has(`${run}:${cycle}`)) {
            assert.deepEqual(
              correctness,
              comparisonSnapshots.get(`${run}:${cycle}`),
              "批量查询必须保留全部记录和图表数值",
            );
          }
          refreshed.correctness = correctness;
          if (process.env.QA_QUERY_PROFILE === "1") {
            refreshed.parseProfile = await client.evaluate(
              "window.qaQueryParse",
            );
          }
          refreshes.push(refreshed);
          assert.equal(refreshed.series, 3);
          assert.equal(refreshed.image, "data:image/png;base64,");
          assert.equal(
            await client.evaluate(`(() => {
        const s=window.qaPinia._s.get('stats');
        return s.charts.getChart().getDom()===document.querySelector('.chart') &&
          s.charts.getProjectChart()?.getDom()===document.querySelector('.project-chart');
      })()`),
            true,
            "图表必须绑定当前页面容器",
          );
          if (cycle === 0) {
            statsMemory = await memory();
            await fs.writeFile(
              path.join(out, `${variant}-stats.png`),
              Buffer.from(
                (await client.send("Page.captureScreenshot", { format: "png" }))
                  .data,
                "base64",
              ),
            );
          }
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
        }
        const retained = await client.evaluate(
          `(() => {const s=window.qaPinia._s.get('stats');return {charts:s.charts!==null,days:Object.keys(s.trendSessions).length}})()`,
        );
        if (variant === "after")
          assert.deepEqual(retained, { charts: false, days: 0 });
        const afterMemoryImmediate = await memory();
        const settleMs = Number(process.env.QA_MEMORY_SETTLE_MS || 0);
        if (settleMs > 0) await pause(settleMs);
        const afterMemory =
          settleMs > 0 ? await memory() : afterMemoryImmediate;
        let features;
        if (process.env.QA_FEATURES === "1" && variant === "after") {
          features = await testFeatures(client);
          await fs.writeFile(path.join(out, "export.html"), features.html);
          delete features.html;
        }
        await client.evaluate(
          `void fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':'startup-memory-qa'},body:JSON.stringify({object:2,method:2})})`,
        );
        client.ws.close();
        await pause(500);
        const record = {
          run,
          variant,
          launchToHomeMs,
          switches,
          refreshes,
          homeMemory,
          statsMemory,
          afterMemory,
          afterMemoryImmediate,
          retained,
          features,
        };
        results.push(record);
        await fs.writeFile(
          path.join(out, "windows-performance.json"),
          JSON.stringify(results, null, 2),
        );
        console.log(
          "PASS",
          variant,
          run,
          Math.round(launchToHomeMs),
          switches.map(Math.round),
        );
      } finally {
        if (client.ws.readyState === WebSocket.OPEN) {
          await client
            .evaluate(
              `void fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':'startup-memory-qa'},body:JSON.stringify({object:2,method:2})})`,
            )
            .catch(() => {});
          client.ws.close();
        }
      }
    }
  }
}

export { connect, until, pause, windowsPath, powershell, ps };

async function testFeatures(client) {
  await client.evaluate(
    `document.querySelector('[aria-label="学习统计"]').click()`,
  );
  await until(
    () =>
      client.evaluate(
        `!!window.qaPinia._s.get('stats').charts?.getChart() && !window.qaPinia._s.get('stats').statsLoading`,
      ),
    "导出前图表",
  );
  await client.evaluate(`(() => {
    const stringify = JSON.stringify;
    JSON.stringify = function(value, ...rest) {
      if (value?.args?.methodID === 250418985) {
        const [filename, html] = value.args.args;
        window.qaExport = {filename, html};
        JSON.stringify = stringify;
        throw Error('QA_EXPORT_CAPTURE');
      }
      return stringify.call(JSON, value, ...rest);
    };
    document.querySelector('.stats-export-button').click();
  })()`);
  await until(() => client.evaluate("!!window.qaExport"), "HTML 生成");
  const exported = await client.evaluate("window.qaExport");
  assert.match(exported.html, /<!doctype html>/i);
  const chartImages = await client.evaluate(`(() => {
    const report = new DOMParser().parseFromString(window.qaExport.html, 'text/html');
    return report.querySelectorAll('img.export-chart-image[src^="data:image/png;base64,"]').length;
  })()`);
  assert.equal(chartImages, 2);
  await client.evaluate(`(() => {
    const app=window.qaPinia._s.get('app');
    app.error='';
    app.selectedDate='2020-02-01';
  })()`);
  await until(
    () =>
      client.evaluate(`(() => {
    const stats=window.qaPinia._s.get('stats');
    return !stats.statsLoading && stats.charts?.getChart() &&
      stats.overviewSessionCount===0 && stats.overviewActualSeconds===0 &&
      stats.charts.getChart().getOption().series
        .filter(series=>series.name!=='计划学习')
        .every(series=>series.data.every(value=>value===0));
  })()`),
    "无记录月份",
  );
  await client.evaluate(`(() => {
    const app=window.qaPinia._s.get('app');
    app.selectedDate=app.todayKey;
  })()`);
  await until(
    () =>
      client.evaluate(`(() => {
    const stats=window.qaPinia._s.get('stats');
    return !stats.statsLoading && stats.charts?.getProjectChart() && stats.overviewSessionCount>0;
  })()`),
    "恢复有记录月份",
  );
  await client.evaluate(`(async () => {
    const app=window.qaPinia._s.get('app');
    const stats=window.qaPinia._s.get('stats');
    app.error='';
    // 每次都让 Vue 提交页面切换，再在加载中离开，验证过期请求不会复活图表。
    for(let i=0;i<3;i++) {
      app.page='today';
      await new Promise(r=>setTimeout(r,80));
      app.page='stats';
      await new Promise(r=>setTimeout(r,80));
    }
    app.page='today';
    await new Promise(r=>setTimeout(r,80));
    await window.qaPinia._s.get('sessions').refreshToday();
    if(stats.charts!==null || Object.keys(stats.trendSessions).length) throw Error('离页清理失败');
  })()`);
  const error = await client.evaluate(`window.qaPinia._s.get('app').error`);
  assert.equal(error, "");
  return {
    exportPngImages: 2,
    rapidNavigation: true,
    emptyMonthAndRestore: true,
    html: exported.html,
  };
}
