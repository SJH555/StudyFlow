const assert = require("node:assert/strict");
const fs = require("node:fs");
const Module = require("node:module");
const path = require("node:path");

const root = path.resolve(__dirname, "../..");
const localRequire = Module.createRequire(path.join(root, "vue/package.json"));
const ts = localRequire("typescript");
Module._extensions[".ts"] = (mod, file) => {
  const result = ts.transpileModule(fs.readFileSync(file, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  });
  mod._compile(result.outputText, file);
};

const pending = [];
function request(date) {
  return new Promise((resolve) => pending.push({ date, resolve }));
}

const apiPath = path.join(root, "vue/src/api.ts");
require.cache[apiPath] = {
  id: apiPath,
  filename: apiPath,
  loaded: true,
  exports: {
    api: { activities: request, sessions: request, addActivity: async () => {} },
  },
};

const { createPinia, setActivePinia } = localRequire("pinia");
const { useAppStore } = require(path.join(root, "vue/src/stores/app.ts"));
const { useActivityStore } = require(path.join(root, "vue/src/stores/activity.ts"));
const { useSessionsStore } = require(path.join(root, "vue/src/stores/sessions.ts"));

const dayA = "2026-01-01";
const dayB = "2026-01-02";

function reply(index, date, row) {
  assert.equal(pending[index].date, date);
  pending[index].resolve([row]);
}

async function run(kind) {
  setActivePinia(createPinia());
  const app = useAppStore();
  const store = kind === "activity" ? useActivityStore() : useSessionsStore();
  const rows = () => kind === "activity" ? store.activities : store.sessions;
  const olderRow = { id: 1 };
  const newerRow = { id: 2, endAt: "2026-01-02T12:00:00" };

  app.selectedDate = dayA;
  const first = store.fetchForDate(dayA);
  app.selectedDate = dayB;
  const second = store.fetchForDate(dayB);
  reply(1, dayB, newerRow);
  await second;
  reply(0, dayA, olderRow);
  assert.deepEqual(await first, [olderRow]);
  assert.deepEqual(rows(), [newerRow], kind + ": late A response replaced B");

  pending.length = 0;
  const earlierRefresh = store.fetchForDate(dayB);
  const laterRefresh = store.fetchForDate(dayB);
  reply(1, dayB, newerRow);
  await laterRefresh;
  reply(0, dayB, olderRow);
  await earlierRefresh;
  assert.deepEqual(rows(), [newerRow], kind + ": older refresh replaced the latest one");

  pending.length = 0;
  if (kind === "activity") {
    store.activityText = "test";
    const adding = store.addActivity();
    await Promise.resolve();
    app.selectedDate = dayA;
    const changing = store.fetchForDate(dayA);
    reply(1, dayA, newerRow);
    await changing;
    reply(0, dayB, olderRow);
    await adding;
    assert.deepEqual(rows(), [newerRow], "post-save response replaced the current date");
  } else {
    app.page = "today";
    app.selectedDate = app.todayKey;
    const today = app.todayKey;
    const refreshing = store.refreshToday();
    const changing = store.fetchForDate(today);
    reply(1, today, newerRow);
    await changing;
    reply(0, today, olderRow);
    assert.deepEqual(await refreshing, [olderRow]);
    assert.deepEqual(rows(), [newerRow], "today refresh replaced a newer selected-date request");

    pending.length = 0;
    const initial = store.loadInitial();
    app.page = "stats";
    app.selectedDate = dayA;
    const navigating = store.fetchForDate(dayA);
    reply(1, dayA, newerRow);
    await navigating;
    reply(0, today, olderRow);
    await initial;
    assert.deepEqual(rows(), [newerRow], "initial load replaced the newly selected date");
  }

  console.log("PASS: " + kind + " date and refresh ordering");
}

run(process.argv[2]).catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
