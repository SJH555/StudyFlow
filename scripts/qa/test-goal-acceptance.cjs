// Exercise the real GoalsPanel setup with an in-memory store; never touch user data.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const root = path.resolve(__dirname, '../..');
const localRequire = createRequire(path.join(root, 'vue/package.json'));
const { parse, compileScript } = localRequire('@vue/compiler-sfc');
const ts = localRequire('typescript');
const vue = localRequire('vue');
const file = path.join(root, 'vue/src/features/goals/GoalsPanel.vue');
const { descriptor } = parse(fs.readFileSync(file, 'utf8'));
const script = compileScript(descriptor, { id: 'goal-acceptance-test' });
const compiled = ts.transpileModule(script.content, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;

async function scenario(order, completed, failure = false, lastGoal = false) {
  const goals = vue.ref([
    { id: 1, deadline: '2026-11-19', createdAt: '2026-11-01', accepted: false },
    ...(!lastGoal ? [
      { id: 2, deadline: '2026-11-20', createdAt: '2026-11-03', accepted: false },
      { id: 3, deadline: '2026-11-21', createdAt: '2026-11-02', accepted: false },
    ] : []),
    { id: 4, deadline: '2026-11-22', createdAt: '2026-11-05', accepted: true },
    { id: 5, deadline: '2026-11-23', createdAt: '2026-11-06', accepted: true },
  ]);
  const byDeadline = accepted => vue.computed(() => goals.value
    .filter(goal => goal.accepted === accepted)
    .sort((a, b) => a.deadline.localeCompare(b.deadline) || a.id - b.id));
  let requests = 0, error;
  const store = {
    activeGoals: byDeadline(false), acceptedGoals: byDeadline(true),
    dueSoonCount: vue.ref(0),
    async accept(id, result) {
      requests++;
      assert.equal(result, completed);
      if (failure) throw Error('验收失败');
      // Match the actual store's full refetch, including changed response order.
      goals.value = goals.value.map(goal => goal.id === id
        ? { ...goal, accepted: true, completed: result } : goal).reverse();
    },
  };
  const mod = { exports: {} };
  new Function('require', 'module', 'exports', compiled)(name => {
    if (name === 'vue') return vue;
    if (name === 'pinia') return { storeToRefs: value => value };
    if (name.endsWith('/stores/app')) return { useAppStore: () => ({ setError: value => { error = value; } }) };
    if (name.endsWith('/stores/goals')) return { useGoalsStore: () => store };
    return {};
  }, mod, mod.exports);
  const panel = mod.exports.default.setup({}, { expose() {} });
  panel.sortOrder.value = order;
  const before = panel.visibleGoals.value.map(goal => goal.id);
  panel.acceptTarget.value = goals.value[0];
  await panel.acceptResult(completed);
  assert.equal(requests, 1);
  assert.equal(panel.showAccepted.value, false, 'acceptance must preserve current list');
  assert.equal(panel.sortOrder.value, order);
  assert.deepEqual(panel.visibleGoals.value.map(goal => goal.id), failure ? before : before.filter(id => id !== 1));
  if (failure) {
    assert.match(error.message, /验收失败/);
    assert.equal(panel.acceptTarget.value.id, 1);
  } else {
    assert.equal(panel.acceptTarget.value, null);
    panel.showAccepted.value = true;
    assert.deepEqual(panel.visibleGoals.value.map(goal => goal.id), order === 'deadline' ? [1, 4, 5] : [5, 4, 1]);
    panel.showAccepted.value = false;
    assert.deepEqual(panel.visibleGoals.value.map(goal => goal.id), before.filter(id => id !== 1));
  }
}

(async () => {
  for (const order of ['deadline', 'created']) {
    for (const completed of [true, false]) await scenario(order, completed);
    await scenario(order, true, true);
    await scenario(order, true, false, true);
  }
  console.log('PASS: acceptance preserves list/sort; both result types, API failure, last goal, and accepted-list sorting');
})().catch(error => { console.error(error); process.exitCode = 1; });
