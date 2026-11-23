// 使用真实 Pinia store 和可延迟的 API，验证活动保存的并发保护。
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

const submissions = [];
let save = async () => {};
let acknowledge = async () => {};
const api = {
  addActivity: async (content) => {
    submissions.push(content);
    await save();
  },
  activities: async () => [],
};

const apiPath = path.join(root, "vue/src/api.ts");
require.cache[apiPath] = {
  id: apiPath,
  filename: apiPath,
  loaded: true,
  exports: { api, acknowledgeReminder: () => acknowledge() },
};
const { createPinia, setActivePinia } = localRequire("pinia");
setActivePinia(createPinia());
const { useAppStore } = require(path.join(root, "vue/src/stores/app.ts"));
const { useActivityStore } = require(path.join(root, "vue/src/stores/activity.ts"));

async function main() {
  const errors = [];
  useAppStore().setError = (error) => errors.push(error);
  const store = useActivityStore();

  // 模拟尚未返回的保存请求以及其间到达的活动提醒。
  const manual = Promise.withResolvers();
  save = () => manual.promise;
  store.openActivity();
  store.activityText = " 读书 ";
  const savingManual = store.addActivity();
  assert.equal(store.activitySaving, true);
  await store.addActivity();
  assert.deepEqual(submissions, ["读书"]);
  store.beginActivityReminder();
  store.activityText = "写笔记";
  manual.resolve();
  await savingManual;
  assert.equal(store.activitySaving, false);
  assert.equal(store.reminderActive, true);
  assert.equal(store.activityText, "写笔记");

  // 提醒提交先确认铃声，等待期间也不能重复提交。
  const reminder = Promise.withResolvers();
  let acknowledgements = 0;
  acknowledge = () => {
    acknowledgements++;
    return reminder.promise;
  };
  const failure = new Error("模拟保存失败");
  save = async () => { throw failure; };
  const savingReminder = store.addActivity();
  await store.addActivity();
  assert.equal(acknowledgements, 1);
  assert.deepEqual(submissions, ["读书"]);
  reminder.resolve();
  await savingReminder;
  assert.deepEqual(submissions, ["读书", "写笔记"]);
  assert.deepEqual(errors, [failure]);
  assert.equal(store.activitySaving, false);
  assert.equal(store.reminderActive, true);
  assert.equal(store.activityText, "写笔记");

  save = async () => {};
  await store.addActivity();
  assert.deepEqual(submissions, ["读书", "写笔记", "写笔记"]);
  assert.equal(store.reminderActive, false);
  assert.equal(store.showActivity, false);
  assert.equal(store.activitySaving, false);
  console.log("PASS: manual and reminder submissions are guarded; failed saves remain retryable");
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
