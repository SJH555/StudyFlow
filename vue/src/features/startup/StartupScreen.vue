<script setup lang="ts">
import { computed } from 'vue';
import type { StartupState } from './types';
const props = defineProps<{ state: StartupState }>();
defineEmits<{ confirm: []; quit: [] }>();
const busy = computed(() => ['checking', 'upgrading'].includes(props.state.phase));
const timezoneBlocked = computed(() => props.state.kind.startsWith('timezone-'));
const title = computed(() => {
  if (props.state.kind === 'timezone-unsupported') return '其他时区暂无法使用';
  if (props.state.kind === 'timezone-unknown') return '无法确认系统时区';
  if (props.state.phase === 'checking') return '正在准备学习空间';
  if (props.state.phase === 'upgrading') return '正在适配新版本，请稍候';
  if (props.state.phase === 'confirmation') return '检测到版本更新';
  if (props.state.kind === 'newer') return '请先更新 StudyFlow';
  if (props.state.kind === 'unrecognized') return '需要检查学习数据';
  return '暂时无法打开学习数据';
});
const description = computed(() => {
  if (props.state.phase === 'checking') return '正在检查数据，请稍候…';
  if (props.state.phase === 'upgrading') return '完成后将自动进入应用，请勿关闭程序。';
  if (props.state.phase === 'confirmation') return '需要调整本地数据格式，以适配新版本。学习记录和设置会保留，开始前将自动备份。';
  return props.state.message;
});
</script>

<template>
  <main class="startup-stage">
    <section class="startup-card" aria-labelledby="startup-title" :aria-busy="busy" data-testid="startup-screen">
      <div class="startup-brand"><img src="/app-icon.png" alt="" /><span>StudyFlow</span></div>
      <h1 id="startup-title">{{ title }}</h1>
      <p class="startup-description" role="status" aria-live="polite">{{ description }}</p>
      <dl v-if="timezoneBlocked" class="startup-facts">
        <div><dt>当前系统时区</dt><dd>{{ state.systemZone || '无法识别' }}</dd></div>
        <div><dt>支持的时区</dt><dd>北京时间（UTC+08:00）</dd></div>
      </dl>
      <dl v-else-if="state.phase === 'blocked'" class="startup-facts">
        <div><dt>当前数据</dt><dd>{{ state.kind === 'baseline' || state.kind === 'historical' ? '尚未登记版本' : `版本 ${state.current}` }}</dd></div>
        <div><dt>{{ state.kind === 'newer' ? '此应用支持' : '目标版本' }}</dt><dd>版本 {{ state.target }}</dd></div>
        <div v-if="state.databasePath" class="startup-path"><dt>数据位置</dt><dd>{{ state.databasePath }}</dd></div>
        <div v-if="state.backupPath" class="startup-path"><dt>升级前备份</dt><dd>{{ state.backupPath }}</dd></div>
      </dl>
      <p v-if="state.kind === 'newer'" class="startup-backup-note">请使用支持数据库版本 {{ state.current }} 或更高版本的应用。</p>
      <details v-if="state.detail" class="startup-details"><summary>查看错误详情</summary><pre>{{ state.detail }}</pre></details>
      <div class="confirm-actions startup-actions">
        <VBtn variant="text" rounded="lg" @click="$emit('quit')">{{ state.phase === 'confirmation' ? '暂时退出' : '退出' }}</VBtn>
        <VBtn v-if="state.phase === 'confirmation' || state.phase === 'upgrading'" variant="tonal" color="primary" rounded="lg" prepend-icon="mdi-database-sync-outline" :disabled="busy" :loading="state.phase === 'upgrading'" @click="$emit('confirm')">确认继续</VBtn>
      </div>
    </section>
  </main>
</template>

<style scoped>
.startup-stage { min-height: calc(100vh - 36px); display: grid; place-items: center; padding: 52px 28px; }
.startup-card { width: min(100%, 620px); padding: 36px 40px 28px; border: 1px solid var(--line, #e8ebf2); border-radius: var(--shape-large, 16px); background: #fff; box-shadow: var(--elevation-panel); }
.startup-brand { display: flex; align-items: center; gap: 9px; font-size: 14px; font-weight: 650; color: var(--ink, #282821); margin-bottom: 28px; }
.startup-brand img { width: 26px; height: 26px; }
h1 { font-size: 24px; line-height: 1.4; font-weight: 650; color: var(--ink, #282821); margin: 0 0 12px; }
.startup-description { color: var(--muted, #6d7481); line-height: 1.8; font-size: 14px; margin-bottom: 24px; }
.startup-facts { background: var(--surface-app, #fff); border: 1px solid var(--line, #e8ebf2); border-radius: 12px; padding: 8px 16px; margin: 0 0 18px; }
.startup-facts > div { display: flex; align-items: baseline; gap: 16px; padding: 10px 0; font-size: 13px; }
.startup-facts dt { flex: 0 0 78px; color: var(--muted, #6d7481); }
.startup-facts dd { margin: 0; color: var(--ink, #282821); overflow-wrap: anywhere; min-width: 0; }
.startup-path { border-top: 1px solid var(--line, #e8ebf2); }
.startup-backup-note { display: flex; align-items: flex-start; gap: 9px; color: var(--muted, #6d7481); font-size: 13px; line-height: 1.8; }
.startup-backup-note .v-icon { color: var(--accent-blue, #2932e1); margin-top: 2px; flex: none; }
.startup-backup-note p { margin: 0; }
.startup-details { font-size: 12px; color: var(--muted, #6d7481); margin-top: 18px; }
.startup-details summary { cursor: pointer; }
.startup-details pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 160px; overflow: auto; background: var(--surface-app, #fff); padding: 12px; border-radius: 8px; margin-top: 10px; user-select: text; }
.startup-actions { display: flex; justify-content: flex-end; gap: 10px; border-top: 1px solid var(--line, #e8ebf2); padding-top: 20px; margin-top: 28px; }
@media (max-width: 660px) { .startup-stage { padding: 24px 16px; } .startup-card { padding: 26px 22px 22px; } }
</style>
