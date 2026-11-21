// 在独立 Windows WebView 中模拟本地跨天，统计真实 Binding 请求。
// 夹具的 after 库在本地日期 +1、+2、+3 天各有一条活动和学习记录，
// 活动内容、学习记录备注及科目依次为 QA next day、QA day two、QA day three。
import assert from "node:assert/strict";
import path from "node:path";
import { spawn } from "node:child_process";
import { connect, until, ps, windowsPath } from "./test-startup-memory-windows.mjs";

const endpoint = "http://127.0.0.1:19349/json/list";
const debugOpen = () => fetch(endpoint, { signal: AbortSignal.timeout(500) })
  .then((response) => response.ok, () => false);
assert.equal(await debugOpen(), false, "调试端口已占用");
spawn(ps, [
  "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
  windowsPath(path.join(process.argv[2], "launch.ps1")), "-Variant", "after",
], { stdio: "ignore" }).unref();

const client = await connect();
const stats = ".stats-date-trigger";
const activity = ".activity-date-trigger";

async function nav(label, selector) {
  const target = JSON.stringify(`[aria-label="${label}"]`);
  await client.evaluate(`document.querySelector(${target}).click()`);
  await until(() => client.evaluate(
    `!!document.querySelector(${JSON.stringify(selector)})`,
  ), label);
}

async function open(selector) {
  await client.evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
  await until(() => client.evaluate(
    "!!document.querySelector('.v-date-picker-month__day--selected')",
  ), "日历展开");
}

async function crossMidnight(expected) {
  await client.evaluate(`window.qaSetClock(${JSON.stringify(expected)} + 'T00:00:01')`);
  await until(() => client.evaluate(
    `window.qaApp.todayKey === ${JSON.stringify(expected)}`,
  ), "现有定时器检测到次日");
}

const snapshot = () => client.evaluate(`({
  today: window.qaApp.todayKey,
  selected: window.qaApp.selectedDate,
  picked: document.querySelector('.v-date-picker-month__day--selected')?.dataset.vDate,
  marker: document.querySelector('.v-date-picker [aria-current=date]')?.closest('[data-v-date]')?.dataset.vDate,
  activity: document.querySelector('.activity-entry b')?.textContent,
  session: window.qaStats.selectedSessions[0]?.note,
})`);

const resetCalls = () => client.evaluate("void (window.qaRequests = [])");
const dateReads = () => client.evaluate(`window.qaRequests.filter((route) =>
  /^(sessions(?:\\/batch)?|activities)\\?/.test(route))`);
const painted = () => client.evaluate(
  "new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve(true))))",
);

try {
  await until(() => client.evaluate(
    "!document.querySelector('.loading') && !!document.querySelector('.today-records-panel')",
  ), "今日页已加载");
  const { today, next, second, third } = await client.evaluate(`(() => {
    const provides = document.querySelector('#app').__vue_app__._context.provides;
    const pinia = Reflect.ownKeys(provides).map(key => provides[key])
      .find(value => value?._s instanceof Map);
    window.qaApp = pinia._s.get('app');
    window.qaStats = pinia._s.get('stats');
    const today = window.qaApp.todayKey;
    const NativeDate = Date;
    const dayAfter = (offset) => {
      const day = new NativeDate(today + 'T12:00:00');
      day.setDate(day.getDate() + offset);
      return new Intl.DateTimeFormat('sv-SE').format(day);
    };
    let now;
    window.qaSetClock = (timestamp) => { now = new NativeDate(timestamp).getTime(); };
    window.qaSetClock(today + 'T23:59:59');
    window.Date = new Proxy(NativeDate, {
      construct(target, args, newTarget) {
        return Reflect.construct(target, args.length ? args : [now], newTarget);
      },
      get(target, key, receiver) {
        return key === 'now' ? (() => now) : Reflect.get(target, key, receiver);
      },
    });
    window.qaRequests = [];
    const fetchOriginal = window.fetch;
    window.fetch = function(url, init) {
      const payload = init?.body ? JSON.parse(init.body) : null;
      // Wails 生成绑定 APIService.Call 的方法编号。
      if (payload?.args?.methodID === 4156410141) {
        window.qaRequests.push(payload.args.args[0]);
      }
      return fetchOriginal.call(this, url, init);
    };
    return { today, next: dayAfter(1), second: dayAfter(2), third: dayAfter(3) };
  })()`);
  await nav("学习统计", stats);
  await open(stats);
  assert.equal((await snapshot()).marker, today);
  await resetCalls();
  await crossMidnight(next);
  await until(async () => {
    const state = await snapshot();
    return state.selected === next && state.session === "QA next day" &&
      state.picked === next && state.marker === next;
  }, "统计选中次日、更新标记及记录");
  const statsReads = await dateReads();
  assert.equal(statsReads.length, 1);
  assert.ok(statsReads[0].startsWith("sessions/batch?"));
  assert.ok(new URLSearchParams(statsReads[0].split("?")[1]).getAll("date").includes(next));
  console.log("PASS: open statistics followed midnight and did not query hidden activities");

  await nav("活动提醒", activity);
  await until(async () => (await snapshot()).activity === "QA next day", "进入活动页读取次日");
  await open(activity);
  await resetCalls();
  await crossMidnight(second);
  await until(async () => {
    const state = await snapshot();
    return state.selected === second && state.activity === "QA day two" &&
      state.picked === second && state.marker === second;
  }, "活动选中次日、更新标记及列表");
  assert.deepEqual(await dateReads(), ["activities?date=" + second]);
  console.log("PASS: open activities followed midnight with one activity query");

  const previous = JSON.stringify(`[data-v-date="${next}"] button`);
  await client.evaluate(`document.querySelector(${previous}).click()`);
  await until(async () => (await snapshot()).activity === "QA next day", "手动选择历史日期");
  await open(activity);
  await resetCalls();
  await crossMidnight(third);
  await until(async () => {
    const state = await snapshot();
    return state.marker === third && state.picked === next;
  }, "保留历史选中日期并更新今天标记");
  await painted();
  assert.equal((await snapshot()).activity, "QA next day");
  assert.deepEqual(await dateReads(), []);
  await nav("学习统计", stats);
  await until(async () => (await snapshot()).session === "QA next day", "统计继续读取历史日期");
  await nav("今日计划", ".today-records-panel");
  await until(() => client.evaluate(
    "document.querySelector('.today-records-panel .record-row b')?.textContent === 'QA day three'",
  ), "返回今日时读取最新日期");
  console.log("PASS: history stayed selected with no midnight reads; Today loaded the latest day");
} finally {
  await client.evaluate(`void fetch('/wails/runtime', {
    method: 'POST',
    headers: {'Content-Type':'application/json','x-wails-client-id':'midnight-qa'},
    body: JSON.stringify({object:2,method:2})
  })`).catch(() => {});
  client.ws.close();
  await until(async () => !(await debugOpen()), "测试窗口退出");
}
