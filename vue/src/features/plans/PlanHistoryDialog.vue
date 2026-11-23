<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { api, type Plan } from "../../api";
import TimelineDialog from "../../shared/TimelineDialog.vue";
import TimelineRow from "../../shared/TimelineRow.vue";
import { weekNames } from "../../shared/constants";
import { plannedStudyMinutes, planEnd } from "../../shared/format";

import { buildPlanHistory } from "./planHistory";

const statusLabels = { stopped: "停用", deleted: "删除" };

const props = defineProps<{ plan: Plan | null }>();
const open = defineModel<boolean>({ default: false });
const versions = ref<Plan[]>([]);
const state = ref<"loading" | "ready" | "error">("loading");

watch(() => props.plan?.id, async (id, _, onCleanup) => {
  versions.value = [];
  state.value = "loading";
  if (!id) return;

  let cancelled = false;
  onCleanup(() => { cancelled = true; });
  try {
    const result = await api.planVersions(id);
    if (cancelled) return;
    versions.value = result;
    state.value = "ready";
  } catch {
    if (!cancelled) state.value = "error";
  }
});

const entries = computed(() => buildPlanHistory(versions.value));
</script>

<template>
  <TimelineDialog
    v-model="open"
    title="版本记录"
    :subtitle="plan ? `${plan.subject} · ${plan.startTime} – ${planEnd(plan)}` : ''"
  >
    <ol v-if="entries.length" class="today-schedule-list plan-history-list">
      <TimelineRow
        v-for="{ version, statusChanges } in entries"
        :key="version.versionId"
        as="li"
        :color="version.color"
        class="plan-history-row"
      >
        <template #time>
          <b><time :datetime="version.effectiveDay" title="版本启用日期">{{ version.effectiveDay }}</time></b>
        </template>
        <div class="plan-card-title">
          <b>{{ version.subject }}</b>
        </div>
        <small>
          {{ version.startTime }} – {{ planEnd(version) }} ·
          时段 {{ version.duration }} 分钟 · 学习 {{ plannedStudyMinutes(version) }} 分钟 ·
          休息 {{ version.breakMinutes }} 分钟
        </small>
        <small>
          {{ version.weekdays === "0,1,2,3,4,5,6" ? "每天"
            : version.weekdays === "1,2,3,4,5" ? "周一至周五"
            : version.weekdays.split(",").map((day) => weekNames[Number(day)]).join("、") }}
        </small>
        <small v-if="version.note" class="plan-history-note">备注 · {{ version.note }}</small>
        <ul v-if="statusChanges.length" class="plan-history-status" role="list">
          <li v-for="{ day, status } in statusChanges" :key="day">
            {{ statusLabels[status] }}时间 <time :datetime="day + 'T00:00'">{{ day }} 00:00</time>
          </li>
        </ul>
      </TimelineRow>
    </ol>
    <div v-else class="today-schedule-empty" role="status">
      {{ state === "loading" ? "正在读取版本记录…" : state === "error" ? "无法读取版本记录" : "没有版本记录" }}
    </div>
  </TimelineDialog>
</template>

<style scoped>
.plan-history-list {
  height: auto;
  max-height: min(70vh, 680px);
  margin: 0;
  padding-top: 5px;
  list-style: none;
}

.plan-history-row.plan-row {
  grid-template-columns: max-content 24px minmax(0, 1fr);
}

.plan-history-row :deep(.plan-time) {
  min-width: 95px;
  justify-content: center;
  padding-block: 0;
}

.plan-history-row :deep(.plan-card-copy small) {
  color: color-mix(in srgb, var(--ink) 62%, white);
}

.plan-history-row :deep(.timeline-line) {
  margin-block: -5px;
}

.plan-history-row:first-child :deep(.timeline-line) {
  top: 50% !important;
  margin-block-start: 0;
}

.plan-history-row:last-child :deep(.timeline-line) {
  bottom: 50% !important;
  margin-block-end: 0;
}

.plan-history-row :deep(.timeline-dot) {
  top: 50%;
  transform: translateY(-50%);
}

.plan-history-row:only-child :deep(.timeline-line) {
  display: none;
}

.plan-history-row :deep(.plan-card-title) {
  flex-wrap: wrap;
}

.plan-history-status {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  margin: var(--space-1) 0 0;
  padding: 0;
  list-style: none;
  font-size: var(--text-caption);
  color: color-mix(in srgb, var(--ink) 62%, white);
}

.plan-history-note {
  white-space: pre-wrap !important;
  overflow-wrap: anywhere;
}
</style>
