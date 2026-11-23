<script setup lang="ts">
import DirectoryStatusButton from "../../shared/DirectoryStatusButton.vue";
import RecordBadge from "../../shared/RecordBadge.vue";
import PlanHistoryDialog from "./PlanHistoryDialog.vue";
import ListActionButton from "../../shared/ListActionButton.vue";
import DirectorySwitch from "../../shared/DirectorySwitch.vue";
import { computed, ref, watch } from "vue";
import { api, type Plan } from "../../api";
import { useAppStore } from "../../stores/app";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlanDraft } from "./usePlanDraft";
import { usePlansForm } from "./usePlans";
import { usePlansView, type PlanTab } from "./usePlansView";
import { weekNames } from "../../shared/constants";
import {
  plannedStudyMinutes,
  formatPlanMinutes,
  formatPlanClock,
  planEnd,
  previewPlanNote,
  formatDateChinese,
} from "../../shared/format";
const { planProjectFilter, openPlanNoteId, togglePlanNote } = usePlanDraft();
const appStore = useAppStore();
const subjectsStore = useSubjectsStore();
const planTab = ref<PlanTab>("active");
const stopped = computed(() => planTab.value === "stopped");
const historyPlan = ref<Plan | null>(null);
const showHistory = ref(false);
let historyTrigger: HTMLElement | null = null;

watch(planTab, () => { planProjectFilter.value = 0; });

function openHistory(plan: Plan, event: MouseEvent) {
  historyTrigger = event.currentTarget as HTMLElement;
  historyPlan.value = plan;
  showHistory.value = true;
}

function restoreHistoryFocus() {
  if (showHistory.value) return;
  historyTrigger?.focus();
  historyTrigger = null;
  historyPlan.value = null;
}

async function changeStatus(plan: Plan) {
  const status = plan.pendingStatus
    ? (plan.status === "active" ? "stopped" : "active")
    : (stopped.value ? "active" : "stopped");
  try {
    const { effectiveDay } = await api.changePlanStatus(plan.id, status);
    await subjectsStore.refreshProjectAndPlan();
    const cancelled = plan.pendingStatus && plan.pendingDay === effectiveDay;
    appStore.notifyTop(cancelled
      ? (status === "active" ? "已撤销停用" : "已撤销启用")
      : `计划于${formatDateChinese(effectiveDay)}${status === "active" ? "启用" : "停用"}`);
  } catch (error) {
    appStore.setError(error);
  }
}

const { beginAddPlan, beginEditPlan, deletePlan } = usePlansForm();
const {
  managedPlans,
  activePlans,
  stoppedPlans,
  planFilterOptions,
  todayPlannedMinutes,
  planBreakdown,
  planPieSegments,
  todayPlanSummary,
} = usePlansView(planTab);
// 饼图悬停项仅计划页使用，作为组件内状态。
const hoveredPlanItem = ref<{ name: string; minutes: number } | null>(null);
</script>

<template>
  <div class="page-heading">
    <div>
      <h1>学习计划</h1>
      <p>按开始时间查看和调整重复计划</p>
    </div>
    <VBtn
      color="primary"
      variant="tonal"
      rounded="lg"
      prepend-icon="mdi-plus"
      @click="beginAddPlan"
      >新建计划</VBtn
    >
  </div>
  <section class="panel plan-manager">
    <div class="plan-toolbar">
      <DirectorySwitch
        class="plan-view-switch"
        v-model="planTab"
        label="学习计划视图"
        :options="[
          { value: 'active', label: '循环计划', count: activePlans.length },
          { value: 'stopped', label: '停用计划', count: stoppedPlans.length },
        ]"
      />
      <VMenu location="bottom end"
        ><template #activator="{ props }"
          ><VBtn
            v-bind="props"
            class="plan-project-select"
            variant="tonal"
            color="primary"
            rounded="lg"
            prepend-icon="mdi-filter-variant"
            append-icon="mdi-chevron-down"
            aria-label="按学习项目筛选计划"
            >{{
              planFilterOptions.find(
                (option) => option.value === planProjectFilter,
              )?.title || "全部项目"
            }}</VBtn
          ></template
        ><VList class="plan-filter-menu" density="compact" rounded="lg"
          ><VListItem
            v-for="option in planFilterOptions"
            :key="option.value"
            :title="option.title"
            :active="planProjectFilter === option.value"
            color="primary"
            rounded="lg"
            @click="planProjectFilter = option.value" /></VList></VMenu
      >
    </div>
    <div v-for="p in managedPlans" :key="p.id" class="managed-plan">
      <span class="manager-color" :style="{ background: p.color }" aria-hidden="true"></span>
      <div class="manager-main">
        <div class="manager-title-line">
          <b>{{ p.subject }}</b>
          <RecordBadge
            v-if="p.pendingLabel"
            class="plan-title-badge"
            icon="mdi-clock-outline"
          >
            {{ p.pendingLabel }} · {{ p.pendingDay }}
          </RecordBadge>
          <VBtn
            v-if="p.note"
            class="plan-note-chip"
            variant="flat"
            rounded="lg"
            size="x-small"
            :aria-expanded="openPlanNoteId === p.id"
            @click.stop="togglePlanNote(p.id)"
            >备注 · {{ previewPlanNote(p.note, 24)
            }}<VTooltip
              activator="parent"
              :model-value="openPlanNoteId === p.id"
              :open-on-hover="false"
              :open-on-focus="false"
              :open-on-click="false"
              location="top"
              transition="scale-transition"
              content-class="subject-tooltip today-note-tooltip"
              ><p>{{ p.note }}</p></VTooltip
            ></VBtn
          >
        </div>
        <small
          >{{ p.startTime }} – {{ planEnd(p) }} · {{ p.duration }} 分钟时段 ·
          学习 {{ plannedStudyMinutes(p) }} 分钟<span v-if="p.breakMinutes">
            · 休息 {{ p.breakMinutes }} 分钟</span
          >
          ·
          {{
            p.weekdays === "0,1,2,3,4,5,6"
              ? "每天"
              : p.weekdays === "1,2,3,4,5"
                ? "周一至周五"
                : p.weekdays
                    .split(",")
                    .map((x) => weekNames[Number(x)])
                    .join("、")
          }}</small
        >
      </div>
      <div class="managed-buttons">
        <template v-if="p.status !== 'deleted'">
          <ListActionButton v-if="!stopped" action="edit" @click="beginEditPlan(p)" />
          <DirectoryStatusButton
            class="plan-list-action plan-status-action"
            :enabled="!stopped"
            :undo="p.pendingStatus"
            :disabled="!p.pendingStatus && (stopped ? p.active : p.pendingLabel === '明日起停用')"
            :aria-label="`${p.pendingStatus ? '撤销' : stopped ? '启用' : '停用'} ${p.subject} ${p.startTime}`"
            @click="changeStatus(p)"
          />
          <ListActionButton action="delete" :aria-label="`删除 ${p.subject} ${p.startTime}`" @click="deletePlan(p)" />
        </template>
        <VBtn
          class="plan-list-action"
          prepend-icon="mdi-history"
          variant="flat"
          color="version-history"
          size="small"
          rounded="lg"
          aria-haspopup="dialog"
          :aria-label="`查看 ${p.subject} ${p.startTime} 的版本记录`"
          @click="openHistory(p, $event)"
        >版本记录</VBtn>
      </div>
    </div>
    <div v-if="!managedPlans.length" class="subtle-empty">
      {{ stopped ? "没有停用计划" : "没有循环计划" }}
    </div>
  </section>
  <PlanHistoryDialog
    v-model="showHistory"
    :plan="historyPlan"
    @after-leave="restoreHistoryFocus"
  />
  <section v-if="planTab === 'active'" class="panel plan-distribution">
    <div class="panel-heading">
      <div>
        <h2>今日学习分配</h2>
        <p>按今天启用的计划统计</p>
      </div>
    </div>
    <div v-if="todayPlanSummary" class="distribution-overview">
      <div class="distribution-metric">
        <span>开始时间</span
        ><strong>{{ formatPlanClock(todayPlanSummary.firstStart) }}</strong>
      </div>
      <div class="distribution-metric">
        <span>结束时间</span
        ><strong>{{ formatPlanClock(todayPlanSummary.lastEnd) }}</strong>
      </div>
      <div class="distribution-metric">
        <span>计划学习</span
        ><strong>{{ formatPlanMinutes(todayPlanSummary.studyMinutes) }}</strong>
      </div>
      <div class="distribution-metric">
        <span>休息</span
        ><strong>{{
          formatPlanMinutes(todayPlanSummary.totalRestMinutes)
        }}</strong>
      </div>
    </div>
    <p v-if="todayPlanSummary" class="distribution-overview-note">
      只统计首个计划开始至最后一个计划结束之间的时间。
    </p>
    <div class="distribution-body">
      <div class="distribution-pie">
        <svg viewBox="0 0 100 100" aria-label="今日计划时长分布">
          <circle
            cx="50"
            cy="50"
            r="40"
            fill="none"
            stroke="#e6eaf2"
            stroke-width="20"
          />
          <circle
            v-for="segment in planPieSegments"
            :key="segment.id"
            cx="50"
            cy="50"
            r="40"
            fill="none"
            :stroke="segment.color"
            stroke-width="20"
            :stroke-dasharray="`${segment.length} ${2 * Math.PI * 40 - segment.length}`"
            :stroke-dashoffset="-segment.offset"
            transform="rotate(-90 50 50)"
            @mouseenter="hoveredPlanItem = segment"
            @mouseleave="hoveredPlanItem = null"
          >
            <title>
              {{ segment.name }} · {{ formatPlanMinutes(segment.minutes) }}
            </title>
          </circle>
        </svg>
        <div class="distribution-center">
          <small>计划学习</small
          ><strong>{{ formatPlanMinutes(todayPlannedMinutes) }}</strong>
        </div>
        <div v-if="hoveredPlanItem" class="pie-tooltip">
          {{ hoveredPlanItem.name }} ·
          {{ formatPlanMinutes(hoveredPlanItem.minutes) }}
        </div>
      </div>
      <div class="distribution-details">
        <div
          v-for="item in planBreakdown"
          :key="item.id"
          class="distribution-row"
        >
          <span class="subject-dot" :style="{ background: item.color }"></span
          ><b>{{ item.name }}</b
          ><span>{{ formatPlanMinutes(item.minutes) }}</span
          ><strong
            >{{
              Math.round((item.minutes / todayPlannedMinutes) * 100)
            }}%</strong
          >
        </div>
        <div v-if="!planBreakdown.length" class="subtle-empty">
          今天没有计划
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.managed-buttons {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-1);
}
.plan-list-action.v-btn {
  block-size: var(--size-list-action);
}

.plan-status-action.v-btn {
  inline-size: var(--size-list-action-width);
}

.managed-plan {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 74px;
  border-bottom: 1px solid #eef0f4;
}

.managed-plan:last-child {
  border-bottom: 0;
}

.manager-color {
  width: 12px;
  height: 36px;
  border-radius: 8px;
  flex: none;
}

.manager-main {
  flex: 1;
  display: grid;
  gap: 4px;
}

.manager-main b {
  font-size: 16px;
}

.manager-main small {
  font-size: 13px;
  color: #7e8798;
}

.plan-inactive {
  font-size: 12px;
  color: #949aa6;
}

.plan-distribution {
  margin-top: 18px;
}

.distribution-body {
  display: grid;
  grid-template-columns: 230px 1fr;
  align-items: center;
  gap: 32px;
  padding: 8px 8px 12px;
}

.distribution-pie {
  width: 205px;
  height: 205px;
  border-radius: 50%;
  display: grid;
  place-items: center;
}

.distribution-pie > div {
  width: 130px;
  height: 130px;
  border-radius: 50%;
  background: var(--surface-card);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
}

.distribution-pie small {
  font-size: 12px;
  color: #8b92a0;
}

.distribution-pie strong {
  font-size: 20px;
  color: #27375d;
}

.distribution-details {
  min-width: 0;
}

.distribution-row {
  display: flex;
  align-items: center;
  gap: 9px;
  min-height: 40px;
  font-size: 13px;
  color: #596273;
}

.distribution-row b {
  flex: 1;
  color: #303744;
  font-size: 14px;
}

.distribution-row strong {
  min-width: 44px;
  text-align: right;
  color: #2932e1;
  font-size: 14px;
}

.distribution-row .subject-dot {
  width: 10px;
  height: 10px;
}

@media (max-width: 1050px) {
  .distribution-body {
    grid-template-columns: 1fr;
    justify-items: center;
  }
}

@media (max-width: 1050px) {
  .distribution-details {
    width: 100%;
  }
}

.distribution-pie {
  position: relative;
  box-shadow: none;
}

.distribution-pie svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
}

.distribution-pie svg circle {
  pointer-events: stroke;
  cursor: default;
}

.distribution-pie > div.distribution-center {
  position: relative;
  z-index: 1;
}

.distribution-pie > div.pie-tooltip {
  position: absolute;
  z-index: 3;
  top: -17px;
  left: 50%;
  transform: translateX(-50%);
  width: max-content;
  max-width: 220px;
  height: auto;
  padding: 7px 11px;
  border-radius: 7px;
  background: #273456;
  color: white;
  font-size: 12px;
  box-shadow: 0 5px 14px #1c2c5a35;
  pointer-events: none;
}

.plan-distribution .panel-heading h2 {
  font-size: 17px;
}

.plan-distribution .panel-heading p {
  font-size: 12px;
}

.distribution-overview {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-top: 20px;
  padding: 18px 0;
  border-top: 1px solid var(--outline-card);
  border-bottom: 1px solid var(--outline-card);
}

.distribution-metric {
  display: grid;
  align-content: center;
  gap: 7px;
  min-width: 0;
  padding: 0 17px;
}

.distribution-metric:first-child {
  padding-left: 0;
}

.distribution-metric:not(:last-child) {
  border-right: 1px solid var(--outline-card);
}

.distribution-metric span {
  color: var(--muted);
  font-size: 12px;
}

.distribution-metric strong {
  color: var(--purple);
  font-size: 19px;
  font-weight: 700;
  line-height: 1.25;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.distribution-overview-note {
  margin: 10px 0 13px;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
}

.distribution-details .distribution-row > span:last-of-type {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1050px) {
  .distribution-overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    row-gap: 15px;
  }
}

@media (max-width: 1050px) {
  .distribution-metric:nth-child(2) {
    border-right: 0;
  }
}

@media (max-width: 1050px) {
  .distribution-metric:nth-child(3) {
    padding-left: 0;
  }
}

.plan-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  padding-bottom: var(--space-4);
  border-bottom: 1px solid var(--outline-card);
}

.plan-toolbar .plan-view-switch {
  margin-bottom: 0;
}

.managed-plan {
  position: relative;
  overflow: visible;
}

.manager-main {
  min-width: 0;
}

.manager-title-line {
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
}

.manager-title-line b {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.plan-project-select.v-btn {
  margin-inline-start: auto;
  width: 260px;
  max-width: 100%;
  justify-content: space-between;
  text-transform: none;
  letter-spacing: 0;
}

:global(.plan-filter-menu.v-list) {
  min-width: 260px;
  padding: 6px;
  background: var(--surface-card);
  border: 1px solid var(--outline-card);
  border-radius: 10px !important;
  box-shadow: 0 8px 24px color-mix(in srgb, var(--ink) 12%, transparent);
}

:global(.plan-filter-menu .v-list-item) {
  margin: 2px 0;
}

.manager-title-line .plan-title-badge {
  min-block-size: 23px;
}

.managed-plan .plan-note-chip.v-btn {
  margin-block-start: 0;
  max-width: 320px;
  min-width: 0;
  height: 23px;
  padding-inline: 8px;
  border-radius: 8px !important;
  box-shadow: none !important;
  font-size: 11px;
  text-transform: none;
  letter-spacing: 0;
}

.managed-plan .plan-note-chip :deep(.v-btn__content) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.plan-manager {
  background: #fff;
}

.plan-distribution {
  background: #fff;
}

.plan-project-select {
  width: 260px;
  max-width: 100%;
  flex: none;
}

.plan-project-select :deep(.v-field) {
  background: #fff;
}

.plan-manager {
  width: 100%;
  max-width: none;
}

.plan-distribution {
  width: 100%;
  max-width: none;
}
</style>
