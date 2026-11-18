<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { Application } from '@wailsio/runtime';
import { StartupService } from '../bindings/exam_notify/desktop';
import { useWindowFrame } from './features/window/useWindowFrame';
import Workspace from './Workspace.vue';
import StartupScreen from './features/startup/StartupScreen.vue';
import type { StartupState } from './features/startup/types';

const state = ref<StartupState>({ phase: 'checking', kind: '', current: 0, target: 0, databasePath: '', backupPath: '', message: '', detail: '' });
const fail = (error: unknown) => {
  state.value = { ...state.value, phase: 'blocked', kind: 'failed', message: '无法完成启动检查，请退出后重新打开 StudyFlow。', detail: String(error) };
};
const { windowsTitlebar, windowsMaximised, windowAction } = useWindowFrame(fail);
onMounted(async () => {
  try { state.value = await StartupService.Status() as StartupState; }
  catch (error) { fail(error); }
});
async function confirm() {
  if (state.value.phase !== 'confirmation') return;
  state.value = { ...state.value, phase: 'upgrading' };
  try { state.value = await StartupService.Confirm() as StartupState; }
  catch (error) { fail(error); }
}
async function quit() { try { await Application.Quit(); } catch (error) { fail(error); } }
</script>

<template>
  <Workspace v-if="state.phase === 'ready'" />
  <div v-else class="startup-shell" :class="{ 'windows-frame': windowsTitlebar, 'windows-maximised': windowsMaximised }">
    <div v-if="windowsTitlebar" class="windows-titlebar">
      <div class="windows-titlebar-drag" @dblclick="windowAction('maximise')"><img src="/app-icon.png" alt="" /><span>StudyFlow</span></div>
      <div class="windows-titlebar-actions">
        <button type="button" aria-label="最小化" @click="windowAction('minimise')">−</button>
        <button type="button" aria-label="最大化或还原" @click="windowAction('maximise')">□</button>
        <button type="button" class="windows-titlebar-close" aria-label="关闭窗口" @click="quit">×</button>
      </div>
    </div>
    <StartupScreen :state="state" @confirm="confirm" @quit="quit" />
  </div>
</template>

<style scoped>
.startup-shell { min-height: 100vh; background: #fff; }
</style>

<style scoped>
.startup-shell .windows-titlebar { background: #fff; }
</style>
