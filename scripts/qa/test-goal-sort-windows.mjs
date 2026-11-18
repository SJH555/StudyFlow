// Run against the temporary Windows QA launcher on the fixed portable directory.
// Only replace the Pinia goal array and accept action in memory; never write goals.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
const pages = await (await fetch('http://127.0.0.1:19347/json/list')).json();
const page = pages.find(p => p.url === 'http://wails.localhost/');
assert.ok(page, 'Windows StudyFlow WebView must be running');
const ws = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let serial = 0;
const pending = new Map();
ws.onmessage = e => {
  const m = JSON.parse(e.data);
  if (!m.id) return;
  const p = pending.get(m.id); pending.delete(m.id);
  m.error ? p.reject(Error(JSON.stringify(m.error))) : p.resolve(m.result);
};
function send(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++serial; pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
}
async function evaluate(expression) {
  const r = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  if (r.exceptionDetails) throw Error(JSON.stringify(r.exceptionDetails));
  return r.result.value;
}
const pause = () => new Promise(resolve => setTimeout(resolve, 250));
async function click(selector, text) {
  await evaluate(`(() => { const e = [...document.querySelectorAll(${JSON.stringify(selector)})].find(e => e.textContent.trim() === ${JSON.stringify(text)}); if (!e) throw Error('Missing control: ' + ${JSON.stringify(text)}); e.click(); })()`);
  await pause();
}
async function state() {
  return evaluate(`({title: document.querySelector('#current-goals-title').textContent.trim(), sort: document.querySelector('.goals-sort-button')?.textContent.trim(), goals: [...document.querySelectorAll('.goal-card h4')].map(e => e.textContent.trim())})`);
}
const output = 'docs/goals-sort-fixed-test-2026-11-18';
const report = [];
try {
  await evaluate(`(() => {
    const p = document.querySelector('#app').__vue_app__._context.provides;
    window.qaStores = Reflect.ownKeys(p).map(k => p[k]).find(v => v?._s)._s;
    window.qaGoalStore = qaStores.get('goals');
    window.qaOriginalGoals = qaGoalStore.goals.slice();
    window.qaOriginalAccept = qaGoalStore.accept;
    window.qaAcceptCalls = [];
    qaGoalStore.accept = async (id, completed) => {
      qaAcceptCalls.push({id, completed});
      qaGoalStore.goals = qaGoalStore.goals.map(g => g.id === id ? {...g, accepted: true, completed} : g).reverse();
    };
    document.querySelector('[aria-label="学情追踪"]').click();
    qaStores.get('review').summaryTab = 'goals';
  })()`);
  await pause();
  for (const order of ['deadline', 'created']) {
    for (const completed of [true, false]) {
      await evaluate(`qaGoalStore.goals = [
        {id: -1, title: 'QA A', deadline: '2026-12-01', createdAt: '2026-11-01T08:00:00+08:00', accepted: false},
        {id: -2, title: 'QA B', deadline: '2026-12-02', createdAt: '2026-11-03T08:00:00+08:00', accepted: false},
        {id: -3, title: 'QA C', deadline: '2026-12-03', createdAt: '2026-11-02T08:00:00+08:00', accepted: false},
        {id: -4, title: 'QA D', deadline: '2026-12-04', createdAt: '2026-11-05T08:00:00+08:00', accepted: true}
      ]`);
      await pause();
      await click('.v-btn-toggle .v-btn', '进行中');
      await click('.goals-sort-button', (await state()).sort);
      await click('.goals-sort-menu .v-list-item', order === 'deadline' ? '截止日期：近到远' : '创建日期：新到旧');
      const before = await state();
      assert.deepEqual(before.goals, order === 'deadline' ? ['QA A', 'QA B', 'QA C'] : ['QA B', 'QA C', 'QA A']);
      await evaluate(`document.querySelector('.goal-card .list-action-button').click()`);
      await pause();
      await click('.plan-confirm-actions .v-btn', completed ? '是' : '否');
      const after = await state();
      assert.equal(after.title, '当前目标');
      assert.equal(after.sort, before.sort);
      assert.deepEqual(after.goals, before.goals.slice(1));
      const call = await evaluate('qaAcceptCalls.at(-1)');
      assert.equal(call.completed, completed);
      await click('.v-btn-toggle .v-btn', '已验收');
      const accepted = await state();
      assert.equal(accepted.sort, before.sort, 'sort dropdown must stay visible in accepted list');
      assert.deepEqual(accepted.goals, order === 'deadline' ? ['QA A', 'QA D'] : ['QA D', 'QA B']);
      // Exercise the dropdown in the accepted view, too.
      await click('.goals-sort-button', accepted.sort);
      await click('.goals-sort-menu .v-list-item', order === 'deadline' ? '创建日期：新到旧' : '截止日期：近到远');
      const changed = await state();
      assert.deepEqual(changed.goals, order === 'deadline' ? ['QA D', 'QA A'] : ['QA B', 'QA D']);
      report.push({order, completed, before, after, accepted, changed});
    }
  }
  const shot = await send('Page.captureScreenshot', { format: 'png' });
  await fs.writeFile(`${output}/accepted-sort.png`, Buffer.from(shot.data, 'base64'));
  await fs.writeFile(`${output}/windows-results.json`, JSON.stringify({passed: true, scenarios: report}, null, 2) + '\n');
  console.log('PASS: real Windows WebView; four acceptance scenarios preserve current list/sort; accepted dropdown visible and functional');
} finally {
  await evaluate(`if (window.qaOriginalGoals) { qaGoalStore.goals = qaOriginalGoals; qaGoalStore.accept = qaOriginalAccept; }`);
  ws.close();
}
