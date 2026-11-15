<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { usePlansStore } from "../../stores/plans";
import { useGoalsStore } from "../../stores/goals";
import { useReviewStore } from "../../stores/review";
import { useSessionsStore } from "../../stores/sessions";
import { usePlanDraft } from "../plans/usePlanDraft";
import { useTodayView } from "./useTodayView";
import TodayScheduleDialog from "./TodayScheduleDialog.vue";
import { useActiveLearning } from "../sessions/useActiveLearning";
import {
  plannedStudyMinutes,
  clockLabel,
  fmt,
  time,
  planEnd,
  previewPlanNote,
  formatDateChinese,
  formatRemainingDays,
} from "../../shared/format";

const appStore = useAppStore();
const { page } = storeToRefs(appStore);
const { enterFocus } = appStore;
const plansStore = usePlansStore();
const { todayPlans, currentPlan } = storeToRefs(plansStore);
const goalsStore = useGoalsStore();
const { activeGoals } = storeToRefs(goalsStore);
const { summaryTab } = storeToRefs(useReviewStore());
const { hasPlanEnded } = plansStore;

const sessionsStore = useSessionsStore();
const {
  showFinishConfirm,
  deleteSessionTarget,
  startingPlanId,
  todayPaused,
  active,
} = storeToRefs(sessionsStore);
const {
  sessionSeconds,
  openManual,
  startCurrent,
  start,
  toggleTodayPause,
  pause,
  resume,
} = sessionsStore;

const { openPlanNoteId, togglePlanNote } = usePlanDraft();
const nearestGoal = computed(() => activeGoals.value[0]);
const nearestGoalDays = computed(() =>
  nearestGoal.value
    ? goalsStore.daysRemaining(nearestGoal.value.deadline)
    : null,
);

const {
  todayPlanListEl,
  showTodaySchedule,
  plannedMinutes,
  unstartedPlanRemainingSeconds,
  locateCurrentPlan,
} = useTodayView();
const {
  activePlan,
  actualSeconds,
  heroBreakMinutes,
  countdownLeft,
  countdownPercent,
  visibleSessions,
} = useActiveLearning();
</script>

<template>
  <div class="page-heading">
    <div><h1>今日计划</h1></div>
    <div class="heading-actions">
      <VBtn
        class="today-pause-button"
        variant="tonal"
        color="primary"
        rounded="lg"
        :prepend-icon="todayPaused ? 'mdi-play-outline' : 'mdi-pause'"
        @click="toggleTodayPause"
        >{{ todayPaused ? "恢复今日计划" : "暂停今日计划" }}</VBtn
      >
    </div>
  </div>
  <section class="hero-card">
    <div class="hero-copy">
      <div v-if="active" class="live-label">
        <span></span>{{ active.pausedAt ? "已暂停" : "正在学习" }}
      </div>
      <h2>
        {{
          active
            ? active.subject
            : currentPlan
              ? currentPlan.subject
              : "准备开始学习"
        }}
      </h2>
      <p class="hero-plan-label">
        {{
          activePlan
            ? activePlan.startTime + " – " + planEnd(activePlan)
            : currentPlan
              ? currentPlan.startTime + " – " + planEnd(currentPlan)
              : "当前没有正在进行的学习计划"
        }}<span v-if="heroBreakMinutes">
          · 含休息 {{ heroBreakMinutes }} 分钟</span
        >
      </p>
      <div v-if="active" class="hero-buttons">
        <button v-if="active.pausedAt" class="hero-button" @click="resume">
          ▶ 继续学习</button
        ><button v-else class="hero-button pause" @click="pause">Ⅱ 暂停</button
        ><button class="hero-button end" @click="showFinishConfirm = true">
          ■ 结束本次学习
        </button>
      </div>
      <VBtn
        v-else
        class="hero-button"
        variant="tonal"
        color="primary"
        rounded="lg"
        prepend-icon="mdi-play-outline"
        :disabled="todayPaused"
        @click="startCurrent"
        >开始学习</VBtn
      >
    </div>
    <div class="hero-countdown">
      <div
        class="countdown-ring"
        :style="{
          background: `conic-gradient(#2932e1 ${countdownPercent}%, #e8ecf7 ${countdownPercent}%)`,
        }"
      >
        <div class="countdown-inner">
          <small>计划学习</small><strong>{{ clockLabel(countdownLeft) }}</strong
          ><span v-if="unstartedPlanRemainingSeconds !== null">
            剩余 {{ clockLabel(unstartedPlanRemainingSeconds) }}
          </span>
        </div>
      </div>
      <button class="focus-entry" :disabled="!active" @click="enterFocus">
        进入专注模式 ↗
      </button>
    </div>
  </section>
  <section class="today-plans-panel">
    <div class="section-head">
      <div>
        <h2>今日安排</h2>
        <span>{{ todayPlans.length }} 个计划时段</span>
      </div>
      <div class="today-plan-actions">
        <VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          size="small"
          prepend-icon="mdi-crosshairs-gps"
          @click="locateCurrentPlan"
          >定位当前</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          size="small"
          prepend-icon="mdi-format-list-bulleted"
          @click="showTodaySchedule = true"
          >查看全部</VBtn
        >
      </div>
    </div>
    <div class="plan-layout">
      <section ref="todayPlanListEl" class="plan-list">
        <div
          v-for="p in todayPlans"
          :key="p.id"
          :data-plan-id="p.id"
          :class="[
            'plan-row',
            { 'current-plan-row': currentPlan?.id === p.id },
          ]"
        >
          <div class="plan-time">
            <b>{{ p.startTime }}</b
            ><span>{{ planEnd(p) }}</span>
          </div>
          <div class="timeline">
            <div class="timeline-line"></div>
            <div class="timeline-dot" :style="{ background: p.color }"></div>
          </div>
          <div class="plan-card">
            <div class="plan-card-main">
              <div class="subject-dot" :style="{ background: p.color }"></div>
              <div class="plan-card-copy">
                <div class="plan-card-title">
                  <b>{{ p.subject }}</b
                  ><VBtn
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
                  >时段 {{ p.duration }} 分钟 · 学习
                  {{ plannedStudyMinutes(p) }} 分钟<span v-if="p.breakMinutes">
                    · 休息 {{ p.breakMinutes }} 分钟</span
                  >
                  · {{ p.startTime }}–{{ planEnd(p) }}</small
                >
              </div>
            </div>
            <VBtn
              class="start-small"
              variant="tonal"
              color="primary"
              rounded="lg"
              size="small"
              :disabled="
                todayPaused ||
                hasPlanEnded(p) ||
                active?.planId === p.id ||
                startingPlanId === p.id
              "
              :title="
                active?.planId === p.id
                  ? active?.pausedAt
                    ? '已暂停'
                    : '进行中'
                  : startingPlanId === p.id
                    ? '正在开始'
                    : todayPaused
                      ? '今日计划已暂停'
                      : hasPlanEnded(p)
                        ? '该时段已结束'
                        : '开始学习'
              "
              @click="start(p)"
              >{{
                active?.planId === p.id
                  ? active?.pausedAt
                    ? "已暂停"
                    : "进行中"
                  : startingPlanId === p.id
                    ? "正在开始…"
                    : "开始学习"
              }}
              <span v-if="active?.planId !== p.id && startingPlanId !== p.id"
                >→</span
              ></VBtn
            >
          </div>
        </div>
        <div v-if="!todayPlans.length" class="empty-card"><b>无计划</b></div>
      </section>
      <aside class="today-side">
        <div class="side-card">
          <div class="side-title">
            <b>计划完成度</b
            ><button class="text-button" @click="page = 'plans'">
              管理计划 <span>↗</span>
            </button>
          </div>
          <div class="side-total">
            <strong
              >{{
                plannedMinutes
                  ? Math.min(
                      100,
                      Math.round((actualSeconds / (plannedMinutes * 60)) * 100),
                    )
                  : 0
              }}%</strong
            ><small>实际 / 计划</small>
          </div>
          <div class="progress-track">
            <div
              :style="{
                width:
                  Math.min(
                    100,
                    plannedMinutes
                      ? (actualSeconds / (plannedMinutes * 60)) * 100
                      : 0,
                  ) + '%',
              }"
            ></div>
          </div>
          <div class="side-foot">
            <span>计划 {{ fmt(plannedMinutes * 60) }}</span
            ><b
              >{{
                plannedMinutes
                  ? Math.min(
                      100,
                      Math.round((actualSeconds / (plannedMinutes * 60)) * 100),
                    )
                  : 0
              }}%</b
            >
          </div>
        </div>
        <div
          class="side-card goal-countdown-card"
          :class="[
            nearestGoal &&
              `goal-countdown-${goalsStore.countdownTone(nearestGoal)}`,
            {
              'goal-countdown-card--empty':
                !nearestGoal || nearestGoalDays === null,
            },
          ]"
        >
          <div class="goal-countdown-header">
            <h2 class="goal-countdown-heading">
              <VIcon icon="mdi-flag-outline" aria-hidden="true" />
              <span>阶段目标倒计时</span>
            </h2>
            <VBtn
              class="goal-countdown-manage"
              variant="text"
              color="primary"
              size="small"
              append-icon="mdi-arrow-top-right"
              @click="
                summaryTab = 'goals';
                page = 'summary';
              "
            >
              管理目标
            </VBtn>
          </div>
          <template v-if="nearestGoal && nearestGoalDays !== null">
            <h3 class="goal-countdown-name">{{ nearestGoal.title }}</h3>
            <VBtn
              v-if="nearestGoal.note"
              class="goal-countdown-detail"
              variant="tonal"
              color="primary"
              rounded="lg"
              size="x-small"
            >
              详情 · {{ previewPlanNote(nearestGoal.note) }}
              <VTooltip
                activator="parent"
                :open-delay="0"
                :close-delay="0"
                :open-on-hover="true"
                :open-on-focus="true"
                :open-on-click="true"
                location="top"
                content-class="goal-countdown-tooltip"
              >
                <p>{{ nearestGoal.note }}</p>
              </VTooltip>
            </VBtn>
            <div class="goal-countdown-footer">
              <div class="goal-countdown-dates">
                <span class="goal-countdown-created">
                  {{
                    formatDateChinese(goalsStore.createdDate(nearestGoal))
                  }}
                  制定
                </span>
                <span class="goal-countdown-deadline">
                  {{ formatDateChinese(nearestGoal.deadline) }}截止
                </span>
              </div>
              <strong class="goal-countdown-days">
                {{ formatRemainingDays(nearestGoalDays) }}
              </strong>
            </div>
          </template>
          <p v-else class="goal-countdown-empty">暂无进行中的阶段目标</p>
        </div>
      </aside>
    </div>
  </section>
  <section class="today-records-panel">
    <div class="section-head records-head">
      <div>
        <h2>学习记录</h2>
        <span>{{ visibleSessions.length }} 条记录</span>
      </div>
      <button class="text-button" @click="openManual">＋ 补充记录</button>
    </div>
    <div class="records-card">
      <div v-for="s in visibleSessions" :key="s.id" class="record-row">
        <div
          class="record-mark"
          :style="{ background: s.color + '20', color: s.color }"
        >
          ◷
        </div>
        <div class="record-name">
          <b>{{ s.subject }}</b
          ><small
            >{{ s.source === "manual" ? "手动补充" : "计时学习" }} ·
            {{ time(s.startAt) }} –
            {{ s.endAt ? time(s.endAt) : "进行中" }}</small
          >
        </div>
        <strong>{{ fmt(sessionSeconds(s)) }}</strong
        ><button
          v-if="s.endAt"
          class="delete-small"
          aria-label="删除学习记录"
          @click="deleteSessionTarget = s"
        >
          <VIcon icon="mdi-trash-can-outline" size="18" />
        </button>
      </div>
      <div
        v-if="!visibleSessions.length"
        class="section-empty today-records-empty"
      >
        <div class="review-empty-illustration">
          <VIcon icon="mdi-book-clock-outline" />
        </div>
        <b>暂无学习记录</b>
      </div>
    </div>
  </section>
  <TodayScheduleDialog v-model="showTodaySchedule" />
</template>

<style scoped>
:global(.today-main .current-plan-row .plan-card) {
  border: 2px solid #2932e1;
  box-shadow: 0 4px 15px #2932e124;
}
:global(.today-main .current-plan-row .plan-time b) {
  color: #2932e1;
}

.empty-card .btn {
  margin-top: 4px;
}

.hero-card {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.hero-copy {
  position: relative;
  z-index: 1;
}
.live-label {
  display: flex;
  align-items: center;
  gap: 7px;
}
.live-label span {
  width: 6px;
  height: 6px;
  background: #b0d1a8;
  border-radius: 50%;
  box-shadow: 0 0 9px #b0d1a8;
}
.hero-copy h2 {
  font-weight: 600;
}
.hero-copy p {
  margin: 0;
  color: #bcb9c6;
  font-size: 11px;
}
.hero-button {
  border: none;
  font-weight: 600;
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.section-head > div {
  display: flex;
  align-items: baseline;
}
.section-head h2 {
  margin: 0;
  font-weight: 600;
}
.section-head span {
  font-size: 10px;
  color: #aaa89f;
}
.text-button {
  border: 0;
  background: none;
}
.text-button span {
  margin-left: 3px;
}
.plan-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.8fr) minmax(235px, 0.9fr);
  gap: 20px;
}
.plan-list {
  padding: 2px 0;
}
.start-small {
  border: 0;
}
.delete-small {
  border: 0;
  background: none;
  color: #c2c0b8;
}
.delete-small:hover {
  color: #d3786f;
}
.side-card {
  border: 1px solid #eeede7;
}
.side-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.side-title > span {
  color: #aaa89f;
}
.side-total {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.side-total strong {
  font-weight: 600;
  letter-spacing: -0.5px;
}
.side-total small {
  color: #aaa89f;
}
.progress-track {
  height: 5px;
  background: #f0eff4;
  border-radius: 5px;
  overflow: hidden;
}
.progress-track > div {
  height: 100%;
  border-radius: 5px;
}
.side-foot {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  color: #aaa89f;
}
.records-head {
  margin-top: 30px;
}
.records-card {
  background: var(--surface-card);
  border: 1px solid #eeede7;
  border-radius: 10px;
  padding: 0 14px;
}
.record-row {
  display: flex;
  align-items: center;
  border-bottom: 1px solid #f3f2ee;
}
.record-row:last-child {
  border: 0;
}
.record-mark {
  display: grid;
  place-items: center;
  border-radius: 8px;
}
.record-name {
  flex: 1;
}
.record-name b {
  display: block;
  font-weight: 600;
}
.record-name small {
  display: block;
  color: #aaa89f;
}
.record-row > strong {
  font-weight: 600;
  color: #66645c;
}
.empty-card {
  min-height: 144px;
  background: var(--surface-card);
  border: 1px dashed #e7e5de;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: #aaa89f;
}
.empty-card b {
  color: #5c5a53;
  font-size: 12px;
}
.empty-card > span {
  font-size: 10px;
}
@media (max-width: 1100px) {
  .plan-layout {
    grid-template-columns: minmax(0, 1.5fr) minmax(210px, 0.8fr);
  }
}
.hero-card {
  height: auto;
  color: #282821;
  border: 1px solid #eeede7;
  box-shadow: none;
}
.hero-copy h2 {
  color: #282821;
}
.live-label {
  color: #77756d;
}
.hero-button {
  margin-top: 12px;
  background: #2932e1;
  color: #fff;
}
.hero-button.end {
  background: #f1f0eb;
  color: #5c5a53;
}
.hero-button.pause {
  background: #f1f0eb;
  color: #5c5a53;
}
.section-head {
  margin: 32px 0 16px;
}
.section-head h2 {
  font-size: 18px;
  line-height: 1.4;
}
.section-head > div {
  gap: 12px;
}
.section-head > div span {
  font-size: 13px;
}
.text-button {
  font-size: 14px;
  padding: 7px 8px;
}
.hero-card {
  padding: 22px 26px;
  border-radius: 13px;
}
.live-label {
  font-size: 13px;
}
.hero-button {
  height: 38px;
  padding: 0 16px;
  font-size: 14px;
  border-radius: 8px;
}
.start-small {
  border-radius: 7px;
}
.delete-small {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  font-size: 20px;
  line-height: 1;
  padding: 4px 7px;
}
.side-card {
  padding: 17px 17px 15px;
  border-radius: 11px;
}
.side-title > b {
  font-size: 14px;
}
.side-title > span {
  font-size: 12px;
}
.side-total {
  margin: 13px 0 10px;
}
.side-total strong {
  font-size: 26px;
}
.side-total small {
  font-size: 12px;
}
.side-foot {
  font-size: 12px;
}
.record-row {
  gap: 12px;
}
.record-mark {
  width: 34px;
  height: 34px;
}
.record-name small {
  margin-top: 3px;
}
.text-button {
  color: #2932e1;
}
.text-button:hover {
  color: #1d25b8;
}
.progress-track > div {
  background: linear-gradient(90deg, #2932e1, #7683ff);
}
.side-foot b {
  color: #2932e1;
}
.start-small {
  background: #eef1ff;
  color: #2932e1;
}
.hero-card {
  min-height: 245px;
}
.hero-copy {
  flex: 1;
  min-width: 0;
}
.hero-copy h2 {
  letter-spacing: -0.2px;
  font-size: 30px;
  line-height: 1.25;
}
.hero-plan-label {
  color: #5d67a0 !important;
  margin-top: 8px !important;
  font-size: 15px !important;
}
.hero-countdown {
  width: 190px;
  flex: none;
  display: grid;
  justify-items: center;
  gap: 10px;
}
.countdown-ring {
  width: 155px;
  height: 155px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  box-shadow: 0 4px 18px #2932e11c;
}
.countdown-inner {
  width: 135px;
  height: 135px;
  border-radius: 50%;
  background: var(--surface-card);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
}
.countdown-inner small {
  font-size: 11px;
  color: #8a91a0;
}
.countdown-inner strong {
  font-size: 29px;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
  color: #26355e;
}
.countdown-inner span {
  font-size: 11px;
  color: #9aa1ae;
}
.focus-entry {
  border: 0;
  background: none;
  color: #2932e1;
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
}
.focus-entry:disabled {
  color: #b7bdca;
  cursor: not-allowed;
}
.record-name b {
  font-size: 16px;
}
.record-name small {
  font-size: 14px;
}
.record-row > strong {
  font-size: 16px;
  font-variant-numeric: tabular-nums;
}
.record-row {
  min-height: 70px;
  height: auto;
}
.start-small {
  height: 34px;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 650;
}
.start-small span {
  font-size: 15px;
  padding-left: 6px;
}
.start-small:disabled {
  background: #f0f1f3;
  color: #a5aab3;
  border: 1px solid #e7e9ee;
  box-shadow: none;
  cursor: not-allowed;
}
.start-small:disabled span {
  color: #a5aab3;
}
.hero-button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.records-head .text-button {
  font-size: 13px;
  font-weight: 650;
}
.hero-copy {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-self: stretch;
  align-items: flex-start;
}
.hero-copy .live-label {
  margin-bottom: 16px;
}
.hero-copy h2 {
  margin: 0 0 12px;
}
.hero-copy .hero-plan-label {
  margin: 0 0 24px !important;
}
.hero-copy > .hero-button {
  width: auto;
  min-width: 142px;
  height: 42px;
  margin: 0;
  border-radius: 999px;
  padding: 0 22px;
  font-size: 14px;
}
.hero-buttons .hero-button {
  width: auto;
  min-width: 142px;
  height: 42px;
  margin: 0;
  border-radius: 999px;
  padding: 0 22px;
  font-size: 14px;
}
.hero-buttons {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.hero-copy > .hero-button:hover {
  transform: translateY(-1px);
}
.hero-buttons .hero-button:hover {
  transform: translateY(-1px);
}
.hero-card {
  background: var(--surface-card);
  border-color: var(--outline-card);
}
.side-card {
  background: var(--surface-card);
  border-color: var(--outline-card);
}

/* 今日页专属规则；通过页面类名约束 App 外壳与弹层。 */

:global(.today-side) {
  display: flex;
  flex-direction: column;
  gap: 11px;
  min-width: 220px;
}

.goal-countdown-card {
  --goal-countdown-accent: var(--purple);
  display: grid;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-3) var(--space-4);
}

.goal-countdown-soon {
  --goal-countdown-accent: rgb(var(--v-theme-warning));
}

.goal-countdown-card--empty {
  flex: 0 1 120px;
  min-height: 0;
  align-content: start;
  grid-template-rows: auto minmax(0, 1fr);
}

.goal-countdown-card--empty .goal-countdown-empty {
  align-self: center;
}

.goal-countdown-overdue {
  --goal-countdown-accent: rgb(var(--v-theme-error));
}

.goal-countdown-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: var(--space-2);
  min-width: 0;
}

.goal-countdown-heading {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  margin: 0;
  color: var(--ink);
  font-size: var(--text-small);
  line-height: 1.4;
}

.goal-countdown-heading .v-icon {
  flex: none;
  color: var(--goal-countdown-accent);
}

.goal-countdown-heading span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.goal-countdown-manage.v-btn {
  min-width: 0;
  padding-inline: var(--space-2);
  text-transform: none;
  letter-spacing: 0;
  white-space: nowrap;
  height: 28px;
}

.goal-countdown-name {
  min-width: 0;
  margin: 0;
  color: var(--ink);
  font-size: var(--text-body);
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.goal-countdown-detail.v-btn {
  width: fit-content;
  max-width: 100%;
  min-width: 0;
  justify-content: flex-start;
  font-size: var(--text-small);
  background: color-mix(
    in srgb,
    var(--goal-countdown-accent) 10%,
    var(--surface-card)
  ) !important;
  color: var(--goal-countdown-accent) !important;
  text-transform: none;
  letter-spacing: 0;
}

.goal-countdown-detail :deep(.v-btn__content) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.goal-countdown-footer {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.goal-countdown-dates {
  display: grid;
  gap: var(--space-1);
  min-width: 0;
}

.goal-countdown-created,
.goal-countdown-deadline {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--muted);
  font-size: var(--text-small);
}

.goal-countdown-days {
  justify-self: end;
  max-width: 100%;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--shape-medium);
  background: color-mix(
    in srgb,
    var(--goal-countdown-accent) 12%,
    var(--surface-card)
  );
  color: var(--goal-countdown-accent);
  font-size: var(--text-heading);
  line-height: 1.25;
  overflow-wrap: anywhere;
}

.goal-countdown-empty {
  margin: 0;
  color: var(--muted);
  font-size: var(--text-small);
}

:global(.goal-countdown-tooltip.v-overlay__content) {
  width: min(300px, calc(100vw - 40px));
  max-width: min(300px, calc(100vw - 40px));
  max-height: 220px;
  overflow-y: auto;
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--outline-card);
  border-radius: var(--shape-medium);
  background: var(--surface-card);
  box-shadow: var(--elevation-item);
  color: var(--ink);
  white-space: normal;
}

:global(.goal-countdown-tooltip p) {
  margin: 0;
  color: var(--ink);
  font-size: var(--text-small);
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

:global(.today-pause-button) {
  font-size: 13px;
  min-width: 140px;
}

:global(.today-main .hero-card),
:global(.today-main .plan-card),
:global(.today-main .side-card),
:global(.today-main .records-card),
:global(.today-main .empty-card),
:global(.today-main .current-plan-row .plan-card) {
  background: #fff;
}

:global(.today-main .today-content) {
  max-width: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 24px 18px 24px 3px;
}

:global(.today-main .page-heading) {
  min-height: 36px;
  margin: 0;
  align-items: center;
  justify-content: space-between;
}

:global(.today-main .hero-card) {
  flex: none;
  min-height: 0;
  height: 220px;
}

:global(.today-plans-panel),
:global(.today-records-panel) {
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 0 16px 8px;
  border: 1px solid var(--outline-card);
  border-radius: 13px;
  background: #fff;
}

:global(.today-plans-panel) {
  flex: none;
  height: max(400px, calc(100vh - 410px));
}

:global(.today-records-panel) {
  flex: none;
  height: 320px;
}

:global(.today-plans-panel .section-head),
:global(.today-records-panel .section-head) {
  flex: none;
  min-height: 47px;
  margin: 0 0 6px;
  border-bottom: 1px solid var(--outline-card);
}

:global(.today-records-panel .section-head) {
  border-bottom: 0;
}

:global(.today-plans-panel .plan-layout) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

:global(.today-plans-panel .plan-list),
:global(.today-records-panel .records-card) {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
  scrollbar-color: var(--outline-card) transparent;
}

:global(.today-plans-panel .plan-list) {
  padding: 0 8px 0 0;
}

:global(.today-plans-panel .plan-row) {
  height: 63px;
  grid-template-columns: 76px 20px minmax(0, 1fr);
  min-width: 0;
}

:global(.today-plans-panel .plan-card) {
  min-width: 0;
  overflow: hidden;
}

:global(.today-plans-panel .plan-card-main),
:global(.today-plans-panel .plan-card-copy),
:global(.today-plans-panel .plan-card-title) {
  min-width: 0;
}

:global(.today-plans-panel .plan-card-main) {
  overflow: hidden;
}

:global(.today-plans-panel .plan-card-copy) {
  overflow: hidden;
  flex: 1;
}

:global(.today-plans-panel .plan-card-copy > small) {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.today-plans-panel .plan-card-title b) {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.today-plans-panel .plan-note-chip) {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
}

:global(.today-plans-panel .plan-note-chip .v-btn__content) {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.today-plans-panel .start-small) {
  flex: none;
  white-space: nowrap;
}

:global(.today-plans-panel .plan-card) {
  height: 54px;
}

:global(.today-plans-panel .plan-time) {
  padding-top: 8px;
  gap: 2px;
}

:global(.today-plans-panel .timeline-dot) {
  top: 12px;
}

:global(.today-plans-panel .plan-row:first-child .timeline-line) {
  top: 12px;
}

:global(.today-records-panel .records-card) {
  flex: 1;
  border: 1px dashed color-mix(in srgb, var(--ink) 24%, var(--outline-card));
  border-radius: 10px;
  padding: 12px 18px;
}

:global(.today-records-panel .today-records-empty) {
  min-height: 100%;
  align-content: center;
}

:global(.today-records-panel .today-records-empty .v-icon) {
  color: var(--purple) !important;
}

:global(.today-records-panel .today-records-empty b) {
  color: var(--ink);
  font-size: 16px;
}

:global(.today-plans-panel .today-side) {
  min-height: 0;
  overflow: visible;
}

:global(.windows-frame .today-plans-panel) {
  height: max(400px, calc(100vh - 448px));
}

:global(.windows-maximised .today-plans-panel) {
  height: max(400px, calc(100vh - 458px));
}

:global(.today-plan-actions) {
  display: flex;
  align-items: center;
  gap: 8px;
}

:global(.today-plan-actions .v-btn) {
  background: var(--purple) !important;
  color: #fff !important;
  text-transform: none;
  letter-spacing: 0;
  font-weight: 600;
}

:global(.today-plan-actions .text-button) {
  font-size: 13px;
  font-weight: 650;
}

:global(.today-main .side-title) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

:global(.today-main .side-title .text-button) {
  font-size: 12px;
  font-weight: 650;
  white-space: nowrap;
}

:global(.today-plan-actions .v-btn .v-btn__content),
:global(.today-plan-actions .v-btn .v-btn__prepend),
:global(.today-plan-actions .v-btn .v-icon) {
  color: #fff !important;
  opacity: 1 !important;
}

:global(.today-main .today-pause-button.v-btn),
:global(.today-main .hero-copy > .hero-button.v-btn),
:global(.today-main .today-plan-actions .v-btn),
:global(.today-main .start-small.v-btn) {
  background: var(--surface-hidden) !important;
  color: var(--purple) !important;
  border-radius: 8px !important;
  box-shadow: none !important;
  text-transform: none;
  letter-spacing: 0;
}

:global(.today-main .today-plan-actions .v-btn .v-btn__content),
:global(.today-main .today-plan-actions .v-btn .v-btn__prepend),
:global(.today-main .today-plan-actions .v-btn .v-icon) {
  color: var(--purple) !important;
}

:global(.today-main .hero-copy > .hero-button.v-btn) {
  min-width: 142px;
  height: 42px;
  margin: 0;
  padding-inline: 18px;
  font-size: 16px;
}

:global(.today-main .plan-note-chip.v-btn) {
  max-width: 320px;
  min-width: 0;
  height: 23px;
  padding-inline: 8px;
  font-size: 11px;
}

:global(.today-main .plan-note-chip .v-btn__content) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.today-main .plan-card .start-small.v-btn) {
  height: 28px !important;
  min-width: 76px;
  padding-inline: 7px !important;
  font-size: 10px !important;
  font-weight: 600;
}

:global(.today-main .plan-card .start-small.v-btn .v-btn__content) {
  padding-left: 0;
  font-size: 10px !important;
  line-height: 1;
}

:global(.today-main .plan-card .start-small.v-btn .v-btn__content > span) {
  padding-left: 2px;
  font-size: 10px !important;
}

:global(.today-main .hero-copy > .hero-button.v-btn .v-btn__prepend .v-icon) {
  font-size: 24px !important;
}

:global(.today-note-tooltip.v-overlay__content) {
  max-width: min(360px, calc(100vw - 40px));
  padding: 10px 13px;
  white-space: normal;
}

:global(.today-note-tooltip span) {
  display: block;
  margin-bottom: 4px;
  color: var(--section-muted);
  font-size: 11px;
}

:global(.today-note-tooltip p) {
  margin: 0;
  color: var(--ink);
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

/* 今日页样式依赖 App 页面标记与浮层类名，均限定在今日模块命名空间内。 */
</style>
