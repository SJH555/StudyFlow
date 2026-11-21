<script setup lang="ts">
import { onUnmounted } from "vue";
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { usePlansStore } from "../../stores/plans";
import { useSessionsStore } from "../../stores/sessions";
import { useStatsStore } from "../../stores/stats";
import { useCharts } from "./useCharts";
import { exportHtml } from "../export/exportHtml";
import SummaryCard from "../../shared/SummaryCard.vue";
import SessionSourceBadge from "../sessions/SessionSourceBadge.vue";
import {
  fmt,
  formatDateChinese,
  plannedStudyMinutes,
  time,
  planEnd,
} from "../../shared/format";
const { selectedDate, todayKey } = storeToRefs(useAppStore());
const { currentPlans } = storeToRefs(usePlansStore());
const { sessionSeconds } = useSessionsStore();
const statsStore = useStatsStore();
const {
  selectedSessions: sessions,
  statsLoading,
  statsReady,
  statsDateMenu,
  statsPeriod,
  overviewPeriod,
  periodDates,
  overviewRangeLabel,
  trendRangeLabel,
  overviewPlannedMinutes,
  overviewActualSeconds,
  overviewPlanCount,
  overviewSessionCount,
  comparisonSubjects,
} = storeToRefs(statsStore);
const {
  pickStatsDate,
  plannedForDate,
  dailyActualSeconds,
  dailyActualTrend,
  completionLabel,
  plannedForPeriod,
  actualForPeriod,
} = statsStore;

// 图表实例属于本页面生命周期：创建后注入 stats store 供加载流程渲染，
// 离开统计页时由 store 的 leaveStatsPage 释放。
const charts = useCharts({
  periodDates,
  statsPeriod,
  comparisonSubjects,
  plannedForDate,
  dailyActualSeconds,
  dailyActualTrend,
  completionLabel,
  plannedForPeriod,
  actualForPeriod,
});
statsStore.attachCharts(charts);
onUnmounted(() => statsStore.leaveStatsPage());
const { chartEl, projectChartEl } = charts;
</script>

<template>
  <div class="page-heading">
    <div><h1>学习统计</h1></div>
    <div class="heading-actions">
      <span v-if="!statsReady" class="stats-date-pending">{{
        formatDateChinese(selectedDate)
      }}</span
      ><VMenu v-else v-model="statsDateMenu" :close-on-content-click="false"
        ><template #activator="{ props }"
          ><VBtn
            v-bind="props"
            variant="outlined"
            color="primary"
            prepend-icon="mdi-calendar-month-outline"
            append-icon="mdi-chevron-down"
            class="stats-date-trigger"
            >{{ formatDateChinese(selectedDate) }}</VBtn
          ></template
        ><VDatePicker
          :key="todayKey"
          :model-value="selectedDate"
          color="primary"
          hide-title
          hide-header
          @update:model-value="pickStatsDate" /></VMenu
      ><VBtn
        class="stats-export-button"
        variant="tonal"
        color="secondary"
        rounded="lg"
        prepend-icon="mdi-export-variant"
        :disabled="statsLoading"
        @click="exportHtml('study')"
        >导出 HTML</VBtn
      >
    </div>
  </div>
  <template v-if="statsReady"
    ><section class="panel overview-panel">
      <div class="panel-heading">
        <div>
          <h2>学习概览</h2>
          <p>{{ overviewRangeLabel }} · 汇总计划与实际学习</p>
        </div>
        <div class="period-switch" role="group" aria-label="概览范围">
          <button
            :class="{ selected: overviewPeriod === 'day' }"
            @click="overviewPeriod = 'day'"
          >
            单日</button
          ><button
            :class="{ selected: overviewPeriod === 'week' }"
            @click="overviewPeriod = 'week'"
          >
            所在周</button
          ><button
            :class="{ selected: overviewPeriod === 'month' }"
            @click="overviewPeriod = 'month'"
          >
            所在月
          </button>
        </div>
      </div>
      <div class="stat-cards">
        <SummaryCard
          label="计划学习"
          icon="mdi-calendar-clock-outline"
          tone="primary"
          tall
        >
          {{ fmt(overviewPlannedMinutes * 60) }}
          <template #detail>来自 {{ overviewPlanCount }} 个计划时段</template>
        </SummaryCard>
        <SummaryCard
          label="实际学习"
          icon="mdi-timer-outline"
          tone="warning"
          tall
        >
          {{ fmt(overviewActualSeconds) }}
          <template #detail>共 {{ overviewSessionCount }} 条学习记录</template>
        </SummaryCard>
        <SummaryCard label="计划完成率" icon="mdi-check" tone="success" tall>
          <template v-if="overviewPlannedMinutes">
            {{
              Math.round(
                (overviewActualSeconds / (overviewPlannedMinutes * 60)) * 100,
              )
            }}<em>%</em>
          </template>
          <template v-else>—</template>
          <template #detail>{{
            overviewPlannedMinutes
              ? "实际时长 ÷ 计划时长"
              : "该范围没有学习计划"
          }}</template>
        </SummaryCard>
      </div>
      <div class="project-compare-panel">
        <div class="panel-heading">
          <div>
            <h2>各学习项目对比</h2>
            <p>同一统计范围内，各项目的计划时长与实际学习时长</p>
          </div>
        </div>
        <div
          v-if="comparisonSubjects.length"
          ref="projectChartEl"
          class="project-chart"
          :style="{
            height: Math.max(240, comparisonSubjects.length * 72 + 80) + 'px',
          }"
        ></div>
        <div v-else class="subtle-empty">所选范围暂无学习项目数据</div>
      </div>
    </section>
    <section class="panel chart-panel trend-wide">
      <div class="panel-heading">
        <div>
          <h2>学习趋势</h2>
          <p>
            {{ trendRangeLabel }} ·
            柱状图对比每日计划与实际，折线追踪实际学习时长
          </p>
        </div>
        <div class="period-switch" role="group" aria-label="趋势范围">
          <button
            :class="{ selected: statsPeriod === 'week' }"
            @click="statsPeriod = 'week'"
          >
            周视图</button
          ><button
            :class="{ selected: statsPeriod === 'month' }"
            @click="statsPeriod = 'month'"
          >
            月视图
          </button>
        </div>
      </div>
      <div ref="chartEl" class="chart"></div>
    </section>
    <section class="panel compare-panel">
      <div class="panel-heading">
        <div>
          <h2>每日计划与实际</h2>
          <p>
            {{ formatDateChinese(selectedDate) }} · 左侧原计划，右侧实际学习
          </p>
        </div>
      </div>
      <div class="compare-columns compare-scroll">
        <div>
          <div class="column-heading">
            原计划 <small>{{ currentPlans.length }} 项</small>
          </div>
          <div class="compare-list-frame">
            <div v-for="p in currentPlans" :key="p.id" class="compare-item">
              <span class="subject-dot" :style="{ background: p.color }"></span>
              <div>
                <b>{{ p.subject }}</b
                ><small
                  >{{ p.startTime }} – {{ planEnd(p)
                  }}<span v-if="p.breakMinutes">
                    · 含休息 {{ p.breakMinutes }} 分钟</span
                  ></small
                >
              </div>
              <strong>{{ fmt(plannedStudyMinutes(p) * 60) }}</strong>
            </div>
            <div v-if="!currentPlans.length" class="subtle-empty">
              无预设计划
            </div>
          </div>
        </div>
        <div>
          <div class="column-heading">
            实际学习 <small>{{ sessions.length }} 项</small>
          </div>
          <div class="compare-list-frame">
            <div v-for="s in sessions" :key="s.id" class="compare-item">
              <span class="subject-dot" :style="{ background: s.color }"></span>
              <div>
                <div class="compare-record-title"><b>{{ s.subject }}</b><SessionSourceBadge :source="s.source" /></div>
                <small
                  >{{ time(s.startAt) }} –
                  {{ s.endAt ? time(s.endAt) : "进行中" }}</small
                >
              </div>
              <strong>{{ fmt(sessionSeconds(s)) }}</strong>
            </div>
            <div v-if="!sessions.length" class="subtle-empty">
              暂无实际学习记录
            </div>
          </div>
        </div>
      </div>
    </section></template
  >
  <div v-else class="stats-initial" role="status" aria-live="polite">
    <section class="panel overview-panel">
      <div class="panel-heading">
        <div>
          <h2>学习概览</h2>
          <p>
            {{
              statsLoading
                ? "正在载入统计数据…"
                : "统计加载失败，请切换页面重试"
            }}
          </p>
        </div>
      </div>
      <div class="stat-cards">
        <SummaryCard
          label="计划学习"
          icon="mdi-calendar-clock-outline"
          tone="primary"
          tall
        >
          —<template #detail>正在读取</template>
        </SummaryCard>
        <SummaryCard
          label="实际学习"
          icon="mdi-timer-outline"
          tone="warning"
          tall
        >
          —<template #detail>正在读取</template>
        </SummaryCard>
        <SummaryCard label="计划完成率" icon="mdi-check" tone="success" tall>
          —<template #detail>正在读取</template>
        </SummaryCard>
      </div>
    </section>
    <section class="panel chart-panel trend-wide">
      <div class="panel-heading">
        <div>
          <h2>学习趋势</h2>
          <p>数据载入后显示图表</p>
        </div>
      </div>
      <div class="stats-chart-pending"></div>
    </section>
    <section class="panel compare-panel">
      <div class="panel-heading">
        <div>
          <h2>每日计划与实际</h2>
          <p>数据载入后显示明细</p>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.compare-record-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 7px;
}
.compare-panel .panel-heading p {
  font-size: 13px;
}
.project-compare-panel .panel-heading p {
  font-size: 13px;
}
.overview-panel > .panel-heading {
  align-items: center;
  gap: 18px;
}

@keyframes stats-shimmer {
  to {
    background-position: -200% 0;
  }
}

.stat-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 13px;
  margin: 25px 0 17px;
}
.compare-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
}
.compare-columns > div + div {
  border-left: 1px solid #f0efe9;
}
.column-heading {
  font-weight: 600;
}
.column-heading small {
  color: #aaa89f;
  font-weight: 400;
  margin-left: 6px;
}
.compare-item {
  display: flex;
  align-items: center;
  border-top: 1px solid #f4f3ef;
}
.compare-item > div {
  flex: 1;
}
.compare-item b {
  display: block;
}
.compare-item strong {
  font-weight: 600;
}
.compare-columns {
  gap: 24px;
  margin-top: 20px;
}
.compare-columns > div + div {
  padding-left: 24px;
}
.column-heading {
  margin-bottom: 11px;
}
.compare-item {
  min-height: 56px;
  gap: 11px;
}
.compare-item b {
  font-size: 13px;
}
.compare-item small {
  display: block;
  color: #aaa89f;
  font-size: 11px;
  margin-top: 3px;
}
.compare-item strong {
  font-size: 12px;
}
.column-heading {
  font-size: 16px;
}
.column-heading small {
  font-size: 13px;
}
.chart {
  width: 100%;
  height: 300px !important;
}
.trend-wide {
  width: 100%;
  margin-top: 16px;
}
.trend-wide .chart {
  height: 335px !important;
}
.period-switch {
  display: inline-flex;
  gap: 3px;
  padding: 3px;
  border: 1px solid #dfe5f2;
  border-radius: 9px;
  background: #f1f4fb;
}
.period-switch button {
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: #697489;
  padding: 7px 15px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.period-switch button.selected {
  background: var(--surface-card);
  color: #2932e1;
  box-shadow: 0 1px 5px #20366e20;
}
.project-compare-panel {
  width: 100%;
  margin-top: 16px;
}
.project-chart {
  width: 100%;
  min-height: 240px;
}
.compare-panel {
  margin-top: 16px;
}
.compare-scroll {
  max-height: 350px;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-right: 8px;
  scrollbar-color: #c9d3ef #f4f6fb;
  scrollbar-width: thin;
}
.compare-scroll .column-heading {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 8px 0;
}
.overview-panel .stat-cards {
  margin: 22px 0 0;
}
.overview-panel .project-compare-panel {
  border-top: 1px solid #e9edf6;
  padding-top: 22px;
  margin-top: 24px;
}
.overview-panel .project-chart {
  margin-top: 12px;
}
.stats-initial .stat-cards {
  margin: 22px 0 0;
}
.stats-initial .compare-panel {
  min-height: 112px;
}
.stats-chart-pending {
  height: 300px;
  border-radius: 10px;
  background: linear-gradient(90deg, #f6f8fc, #eef3fb, #f6f8fc);
  background-size: 200% 100%;
  animation: stats-shimmer 1.8s ease-in-out infinite;
}
.stats-date-pending {
  min-width: 190px;
  height: 40px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  border: 1px solid #e3e7ef;
  border-radius: 9px;
  background: var(--surface-card);
  color: #454b58;
  font-size: 14px;
}
.compare-panel {
  border-color: #e3eaf6;
}
.compare-scroll .column-heading {
  background: var(--surface-compare);
}
.overview-panel {
  background: #fff;
}
.project-compare-panel {
  background: #fff;
}
.chart-panel {
  background: #fff;
}
.stats-date-trigger {
  background: #fff;
  text-transform: none;
  letter-spacing: 0;
}
.compare-panel .compare-scroll > div {
  min-width: 0;
}
.compare-panel .compare-scroll .column-heading {
  padding: 0;
  margin-bottom: 10px;
}
.compare-panel .compare-list-frame {
  padding: 8px 14px;
  border: 1px dashed color-mix(in srgb, var(--ink) 24%, var(--outline-card));
  border-radius: 10px;
}
.compare-panel .compare-list-frame .compare-item:last-of-type {
  border-bottom: 0;
}
.compare-panel {
  background: #fff;
}
.compare-panel .compare-scroll {
  align-items: stretch;
  max-height: none;
  overflow: visible;
  padding-right: 0;
}
.compare-panel .compare-scroll .column-heading {
  position: static;
  background: #fff;
}
.compare-panel .compare-list-frame {
  height: 300px;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
  scrollbar-color: var(--outline-card) transparent;
  background: #fff;
}
.stats-export-button.v-btn {
  background: #f1f2f4 !important;
  color: #505866 !important;
  border: 1px solid #e0e3e9 !important;
  box-shadow: none !important;
  text-transform: none;
  letter-spacing: 0;
}

.overview-panel :deep(.summary-card--primary) {
  --summary-accent: var(--color-plan-learning);
}

.overview-panel > .stat-cards {
  gap: var(--space-3);
}

.compare-list-frame > .compare-item:first-child {
  border-top-color: transparent;
}
</style>
