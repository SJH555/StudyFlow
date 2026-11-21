// 在 Windows 正式构建的 WebView 中点击日期，并控制真实 Binding 响应的交付顺序。
// 使用 prepare-startup-memory.py 生成 before/after 隔离夹具，传入夹具目录作为第一个参数。
// 两份测试库均在当月 5 日、6 日和今日各准备一条活动和已结束的学习记录。
// 活动内容、学习记录备注和对应科目名称分别为 QA date A、QA date B、QA today。
import assert from "node:assert/strict";
import path from "node:path";
import { spawn } from "node:child_process";
import { connect, until, pause, ps, windowsPath } from "./test-startup-memory-windows.mjs";

// 对应 Wails 生成绑定中的 APIService.Call。
const bindingId = 4156410141;
const endpoint = "http://127.0.0.1:19349/json/list";
const debugOpen = () => fetch(endpoint, { signal: AbortSignal.timeout(500) })
  .then((response) => response.ok, () => false);

for (const variant of (process.env.QA_VARIANTS || "before,after").split(",")) {
  assert.equal(await debugOpen(), false, "调试端口已占用");
  spawn(ps, [
    "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
    windowsPath(path.join(process.argv[2], "launch.ps1")), "-Variant", variant,
  ], { stdio: "ignore" }).unref();
  const client = await connect();
  try {
    await until(() => client.evaluate(
      "!!document.querySelector('.today-records-panel .record-row')",
    ), "今日记录就绪");

    const { dayA, dayB, today } = await client.evaluate(`(() => {
      const app = document.querySelector('#app').__vue_app__;
      const pinia = Object.values(app._context.provides).find((value) => value?._s instanceof Map)
        ?? Object.getOwnPropertySymbols(app._context.provides)
          .map((key) => app._context.provides[key]).find((value) => value?._s instanceof Map);
      window.qaDateApp = pinia._s.get('app');
      const today = window.qaDateApp.todayKey;
      const month = today.slice(0, 7);
      return {today, dayA: month + '-05', dayB: month + '-06'};
    })()`);
    assert.ok(![dayA, dayB].includes(today), "测试日期须与今日不同");

    await client.evaluate(`(() => {
      window.qaHeld = {};
      window.qaGate = new Set();
      const fetchOriginal = window.fetch;
      window.fetch = async function(url, options) {
        const data = options?.body ? JSON.parse(options.body) : null;
        const route = data?.args?.methodID === ${bindingId} ? data.args.args[0] : null;
        const response = await fetchOriginal.call(this, url, options);
        const match = [...window.qaGate].find((key) => route === key || route?.startsWith(key + "?"));
        if (match) {
          await new Promise((resolve) => (window.qaHeld[match] = resolve));
        }
        return response;
      };
      document.querySelector('[aria-label="活动提醒"]').click();
    })()`);

    async function pick(date) {
      await client.evaluate("document.querySelector('.activity-date-trigger').click()");
      const selector = `[data-v-date="${date}"] button`;
      await until(() => client.evaluate(
        `!!document.querySelector(${JSON.stringify(selector)})`,
      ), `日期按钮 ${date}`);
      await client.evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
      await until(() => client.evaluate(
        "document.querySelector('.activity-date-trigger').getAttribute('aria-expanded') === 'false'",
      ), "日期菜单关闭");
    }

    async function hold(date) {
      const routes = [`activities?date=${date}`];
      await client.evaluate(`window.qaGate = new Set(${JSON.stringify(routes)})`);
      await pick(date);
      await until(() => client.evaluate(
        `Object.keys(window.qaHeld).length === 1`,
      ), `${date} 活动响应已暂存`);
    }

    async function release() {
      await client.evaluate(`(() => {
        window.qaGate.clear();
        Object.values(window.qaHeld).forEach((release) => release());
        window.qaHeld = {};
      })()`);
      // 给真实 Binding 包装和 Vue DOM 提交留出两个绘制周期。
      await client.evaluate(
        "new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve(true))))",
      );
    }

    const snapshot = () => client.evaluate(`(() => ({
      date: window.qaDateApp.selectedDate,
      activity: document.querySelector('.activity-entry b')?.textContent,
      todayRow: document.querySelector('.today-records-panel .record-row b')?.textContent,
    }))()`);

    await hold(dayA);
    await pick(dayB);
    await until(async () => {
      const state = await snapshot();
      return state.date === dayB && state.activity === "QA date B";
    }, "B 日已更新");
    await release();
    const state = await snapshot();
    assert.equal(state.date, dayB);
    assert.equal(state.activity, variant === "before" ? "QA date A" : "QA date B");
    console.log(variant, state);

    if (variant === "after") {
      await pick(dayA);
      // 进入统计后暂存包含日期明细的批量响应，再返回今日。
      const sessionRoute = "sessions/batch";
      await client.evaluate(`window.qaGate = new Set([${JSON.stringify(sessionRoute)}])`);
      await client.evaluate(`document.querySelector('[aria-label="学习统计"]').click()`);
      await until(() => client.evaluate(
        `typeof window.qaHeld[${JSON.stringify(sessionRoute)}] === 'function'`,
      ), "历史学习记录响应已暂存");
      await client.evaluate(`document.querySelector('[aria-label="今日计划"]').click()`);
      await until(async () => {
        const state = await snapshot();
        return state.date === today && state.todayRow === "QA today";
      }, "今日记录已恢复");
      await release();
      assert.equal((await snapshot()).todayRow, "QA today");
      console.log("PASS: late historical response did not replace visible today records");
    }
  } finally {
    await client.evaluate(`void fetch('/wails/runtime', {
      method: 'POST',
      headers: {'Content-Type':'application/json','x-wails-client-id':'date-order-qa'},
      body: JSON.stringify({object:2,method:2})
    })`).catch(() => {});
    client.ws.close();
    await until(async () => !(await debugOpen()), "测试窗口退出");
  }
  await pause(250);
}
