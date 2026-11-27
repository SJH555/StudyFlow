// Run from WSL after installing vue dependencies. Uses real draft/store logic.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Module = require('node:module');
const path = require('node:path');
const root = path.resolve(__dirname, '../..');
const localRequire = Module.createRequire(path.join(root, 'vue/package.json'));
const ts = localRequire('typescript');
Module._extensions['.ts'] = (mod, file) => {
  const result = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  });
  mod._compile(result.outputText, file);
};
let requests = 0;
let conflict = false;
let warnings = false;
const submitted = [];
const api = {
  addSession: async (data) => {
    requests++;
    submitted.push(data);
    if (conflict) throw new Error('与已有学习记录时间冲突');
    if (warnings && !data.planConflictToken) {
      return { planConflict: { message: '占用数学计划', token: 'confirm-span' } };
    }
    return { id: requests, duration: 600 };
  },
  sessions: async () => [],
};
const apiPath = path.join(root, 'vue/src/api.ts');
require.cache[apiPath] = {
  id: apiPath, filename: apiPath, loaded: true,
  exports: { api, recordClientError: async () => {}, acknowledgeReminder: async () => {} },
};
const { createPinia, setActivePinia } = localRequire('pinia');
setActivePinia(createPinia());
const { useAppStore } = require(path.join(root, 'vue/src/stores/app.ts'));
const { useSessionsStore } = require(path.join(root, 'vue/src/stores/sessions.ts'));
const { useManualDraft } = require(path.join(root, 'vue/src/features/sessions/useManualDraft.ts'));

(async () => {
  const app = useAppStore(), sessions = useSessionsStore(), draft = useManualDraft();
  draft.manualDate.value = app.todayKey;
  draft.manualSubjects.value = [{ id: 1, name: '数学' }];
  draft.manualSubject.value = 1;
  draft.manualStartHour.value = '09';
  draft.manualStartMinute.value = '00';
  draft.manualEndHour.value = '09';
  draft.showManual.value = true;
  for (const minute of ['00', '05', '09']) {
    draft.manualEndMinute.value = minute;
    assert.ok(draft.manualTimeError.value);
    await sessions.addManual();
    assert.equal(requests, 0);
    assert.equal(draft.showManual.value, true);
  }
  draft.manualEndMinute.value = '10';
  assert.equal(draft.manualDurationMinutes.value, 10);
  assert.equal(draft.manualTimeError.value, '');
  await sessions.addManual();
  assert.equal(requests, 1);
  assert.equal(draft.showManual.value, false);

  draft.showManual.value = true;
  draft.manualEndMinute.value = '30';
  conflict = true;
  await sessions.addManual();
  assert.equal(requests, 2);
  assert.equal(draft.showManual.value, true);
  assert.match(app.topNotice, /时间冲突/);
  conflict = false;
  warnings = true;
  await sessions.addManual();
  assert.equal(requests, 3);
  assert.equal(draft.manualPlanConflict.value.token, 'confirm-span');
  await sessions.addManual();
  assert.equal(requests, 3, 'plain Enter must not silently confirm');
  draft.manualEndMinute.value = '40';
  assert.equal(draft.manualPlanConflict.value, null, 'editing clears previous confirmation');
  await sessions.confirmManualPlanConflict();
  assert.equal(requests, 3);
  await sessions.addManual();
  await sessions.confirmManualPlanConflict();
  assert.equal(requests, 5);
  assert.equal(submitted[4].planConflictToken, 'confirm-span');
  assert.equal(draft.showManual.value, false);
  console.log('PASS: minimum duration, hard conflict, explicit confirmation and edit invalidation');
})().catch(error => { console.error(error); process.exitCode = 1; });
