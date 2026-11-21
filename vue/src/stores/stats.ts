import { computed, nextTick, ref, shallowRef, watch } from "vue";
import { defineStore } from "pinia";
import { api, type Session } from "../api";
import {
  dateKey,
  formatDateChinese,
  parseDateKey,
  plannedStudyMinutes,
} from "../shared/format";
import { useAppStore } from "./app";
import { usePlansStore } from "./plans";
import { useSubjectsStore } from "./subjects";
import { useSessionsStore } from "./sessions";
import type { useCharts } from "../features/stats/useCharts";

// 学习统计：按周/月/日聚合计划与实际学习数据，为图表与导出提供数据源。
// 图表实例由页面层创建后通过 attachCharts 注入，store 不直接持有 DOM。
export const useStatsStore = defineStore("stats", () => {
  const appStore = useAppStore();
  const plansStore = usePlansStore();
  const subjectsStore = useSubjectsStore();
  const sessionsStore = useSessionsStore();

  const statsPeriod = ref<"week" | "month">("week");
  const overviewPeriod = ref<"day" | "week" | "month">("day");
  const trendSessions = ref<Record<string, Session[]>>({});
  const comparisonSessions = ref<Record<string, Session[]>>({});
  // 趋势的周/月范围始终包含所选日期，明细复用同一批学习记录。
  const selectedSessions = computed(
    () => trendSessions.value[appStore.selectedDate] ?? [],
  );
  const statsLoading = ref(false);
  const statsReady = ref(false);

  // 统计页日期选择下拉开关。
  const statsDateMenu = ref(false);

  // 图表实例注入点（useCharts 由页面层创建，持有 DOM 引用；离开统计页时释放）。
  const charts = shallowRef<ReturnType<typeof useCharts> | null>(null);

  function attachCharts(instance: ReturnType<typeof useCharts>) {
    charts.value = instance;
  }

  // 窗口尺寸变化/应用卸载时由生命周期层调用，转发给页面注入的图表实例。
  function resizeCharts() {
    charts.value?.resizeCharts();
  }

  function disposeCharts() {
    charts.value?.disposeCharts();
  }

  // 各阶段仅保留最近一次耗时，便于桌面实测时区分查询、调度与同步绘制。
  async function measureStage<T>(
    name: string,
    work: () => T | Promise<T>,
  ): Promise<T> {
    const started = performance.now();
    try {
      return await work();
    } finally {
      const key = `studyflow:stats:${name}`;
      performance.clearMeasures(key);
      performance.measure(key, { start: started, end: performance.now() });
    }
  }

  // 绘制帧调度：等一次重绘后继续，避免图表在元素未布局时初始化。
  async function afterPaint() {
    await nextTick();
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => setTimeout(resolve, 0)),
    );
  }

  // 让出主线程后再继续，避免阻塞 UI；不支持 requestIdleCallback 时降级为固定延时。
  function whenIdle() {
    return new Promise<void>((resolve) => {
      if ("requestIdleCallback" in window)
        window.requestIdleCallback(() => resolve(), { timeout: 1200 });
      else setTimeout(resolve, 120);
    });
  }

  function datesForPeriod(period: "week" | "month") {
    const chosen = new Date(appStore.selectedDate + "T12:00:00"),
      out: Date[] = [];
    if (period === "week") {
      const monday = new Date(chosen);
      // 周起点为周一：周日 getDay()=0，通过 +6 再取模把周日归到上一周的末尾。
      monday.setDate(chosen.getDate() - ((chosen.getDay() + 6) % 7));
      for (let i = 0; i < 7; i++) {
        const d = new Date(monday);
        d.setDate(monday.getDate() + i);
        out.push(d);
      }
    } else {
      const first = new Date(chosen.getFullYear(), chosen.getMonth(), 1);
      for (
        let d = new Date(first);
        d.getMonth() === chosen.getMonth();
        d.setDate(d.getDate() + 1)
      )
        out.push(new Date(d));
    }
    return out;
  }

  const periodDates = computed(() => datesForPeriod(statsPeriod.value));

  const comparisonDates = computed(() =>
    overviewPeriod.value === "day"
      ? [new Date(appStore.selectedDate + "T12:00:00")]
      : datesForPeriod(overviewPeriod.value),
  );

  function plannedForDate(date: Date, subjectId?: number) {
    const wd = date.getDay();
    return plansStore.plans
      .filter(
        (p) =>
          p.active &&
          p.weekdays.split(",").map(Number).includes(wd) &&
          (subjectId === undefined || p.subjectId === subjectId),
      )
      .reduce((n, p) => n + plannedStudyMinutes(p), 0);
  }

  function dailyActualSeconds(date: Date) {
    return (trendSessions.value[dateKey(date)] || []).reduce(
      (total, session) => total + sessionsStore.sessionSeconds(session),
      0,
    );
  }

  function dailyActualTrend(date: Date) {
    const actual = dailyActualSeconds(date);
    // 未来日期且无记录时返回 null，让折线断开而不是画成 0。
    return dateKey(date) > appStore.todayKey && actual === 0
      ? null
      : actual / 3600;
  }

  function completionLabel(plannedSeconds: number, actualSeconds: number) {
    return plannedSeconds > 0
      ? `${new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 1 }).format((actualSeconds / plannedSeconds) * 100)}%`
      : "—（无计划）";
  }

  function actualForPeriod(subjectId?: number) {
    return comparisonDates.value
      .flatMap((d) => comparisonSessions.value[dateKey(d)] || [])
      .filter((s) => subjectId === undefined || s.subjectId === subjectId)
      .reduce((n, s) => n + sessionsStore.sessionSeconds(s), 0);
  }

  function plannedForPeriod(subjectId?: number) {
    return comparisonDates.value.reduce(
      (n, d) => n + plannedForDate(d, subjectId),
      0,
    );
  }

  const overviewRangeLabel = computed(() =>
    overviewPeriod.value === "day"
      ? formatDateChinese(appStore.selectedDate)
      : overviewPeriod.value === "month"
        ? `${appStore.selectedDate.slice(0, 4)}年${Number(appStore.selectedDate.slice(5, 7))}月`
        : `${formatDateChinese(dateKey(comparisonDates.value[0]))}—${formatDateChinese(dateKey(comparisonDates.value[6]))}`,
  );

  const trendRangeLabel = computed(() =>
    statsPeriod.value === "month"
      ? `${appStore.selectedDate.slice(0, 4)}年${Number(appStore.selectedDate.slice(5, 7))}月`
      : `${formatDateChinese(dateKey(periodDates.value[0]))}—${formatDateChinese(dateKey(periodDates.value[6]))}`,
  );

  const overviewPlannedMinutes = computed(() => plannedForPeriod());

  const overviewActualSeconds = computed(() => actualForPeriod());

  const overviewPlanCount = computed(() =>
    comparisonDates.value.reduce(
      (count, date) =>
        count +
        plansStore.plans.filter(
          (p) =>
            p.active &&
            p.weekdays.split(",").map(Number).includes(date.getDay()),
        ).length,
      0,
    ),
  );

  const overviewSessionCount = computed(() =>
    comparisonDates.value.reduce(
      (count, date) =>
        count + (comparisonSessions.value[dateKey(date)] || []).length,
      0,
    ),
  );

  const comparisonSubjects = computed(() =>
    subjectsStore.allSubjects.filter(
      (s) => plannedForPeriod(s.id) > 0 || actualForPeriod(s.id) > 0,
    ),
  );

  let statsLoadSequence = 0,
    trendLoadSequence = 0,
    overviewLoadSequence = 0,
    refreshSequence = 0;

  async function loadStats() {
    const sequence = ++statsLoadSequence;
    const trendDays = periodDates.value.map(dateKey),
      overviewDays = comparisonDates.value.map(dateKey);
    const dates = [...new Set([...trendDays, ...overviewDays])];
    const all = await measureStage("queries", () => api.sessionsForDates(dates));
    // 用递增序号判断结果是否已被更新的请求取代，避免异步竞态覆盖新数据。
    if (sequence !== statsLoadSequence || appStore.page !== "stats") return;
    if (
      trendDays.join() !== periodDates.value.map(dateKey).join() ||
      overviewDays.join() !== comparisonDates.value.map(dateKey).join()
    )
      return loadStats();
    trendSessions.value = Object.fromEntries(
      trendDays.map((day) => [day, all[day]]),
    );
    comparisonSessions.value = Object.fromEntries(
      overviewDays.map((day) => [day, all[day]]),
    );
    statsReady.value = true;
    await measureStage("layout", afterPaint);
    if (sequence !== statsLoadSequence || appStore.page !== "stats") return;
    await measureStage("idle-project", whenIdle);
    if (sequence !== statsLoadSequence || appStore.page !== "stats") return;
    await measureStage("draw-project", () => charts.value?.renderProjectChart());
    await measureStage("idle-trend", whenIdle);
    if (sequence !== statsLoadSequence || appStore.page !== "stats") return;
    await measureStage("draw-trend", () => charts.value?.renderChart());
  }

  // 统计页刷新：先更新科目和计划，再批量读取图表和明细共用的学习记录。
  async function refreshStatsPage() {
    const sequence = ++refreshSequence;
    cancelStats();
    statsLoading.value = true;
    statsReady.value = false;
    try {
      await afterPaint();
      if (sequence !== refreshSequence || appStore.page !== "stats") return;
      const [, currentPlans] = await measureStage("base-queries", () =>
        Promise.all([subjectsStore.fetchAll(), api.plans()]),
      );
      if (sequence !== refreshSequence || appStore.page !== "stats") return;
      plansStore.setPlans(currentPlans);
      await loadStats();
    } finally {
      if (sequence === refreshSequence && appStore.page === "stats")
        statsLoading.value = false;
    }
  }

  async function refreshTrendPeriod() {
    const sequence = ++trendLoadSequence,
      refresh = refreshSequence,
      days = periodDates.value.map(dateKey);
    const entries = await api.sessionsForDates(days);
    if (
      sequence !== trendLoadSequence ||
      refresh !== refreshSequence ||
      appStore.page !== "stats"
    )
      return;
    trendSessions.value = entries;
    await afterPaint();
    if (
      sequence !== trendLoadSequence ||
      refresh !== refreshSequence ||
      appStore.page !== "stats"
    )
      return;
    charts.value?.renderChart();
  }

  async function refreshOverviewPeriod() {
    const sequence = ++overviewLoadSequence,
      refresh = refreshSequence,
      days = comparisonDates.value.map(dateKey);
    const entries = await api.sessionsForDates(days);
    if (
      sequence !== overviewLoadSequence ||
      refresh !== refreshSequence ||
      appStore.page !== "stats"
    )
      return;
    comparisonSessions.value = entries;
    await afterPaint();
    if (
      sequence !== overviewLoadSequence ||
      refresh !== refreshSequence ||
      appStore.page !== "stats"
    )
      return;
    charts.value?.renderProjectChart();
  }

  watch(statsPeriod, async () => {
    if (appStore.page !== "stats" || !statsReady.value) return;
    try {
      await refreshTrendPeriod();
    } catch (e) {
      appStore.setError(e);
    }
  });

  watch(overviewPeriod, async () => {
    if (appStore.page !== "stats" || !statsReady.value) return;
    try {
      await refreshOverviewPeriod();
    } catch (e) {
      appStore.setError(e);
    }
  });

  // 作废进行中的加载序列（防异步竞态覆盖新数据）。
  function cancelStats() {
    statsLoadSequence++;
    trendLoadSequence++;
    overviewLoadSequence++;
  }

  // 离开统计页：作废进行中的加载、复位加载态并释放图表实例。
  function leaveStatsPage() {
    refreshSequence++;
    cancelStats();
    statsLoading.value = false;
    disposeCharts();
    charts.value = null;
    trendSessions.value = {};
    comparisonSessions.value = {};
    statsReady.value = false;
  }

  function pickStatsDate(value: unknown) {
    const day = parseDateKey(value);
    if (day) {
      appStore.selectedDate = day;
      statsDateMenu.value = false;
    }
  }

  return {
    statsPeriod,
    overviewPeriod,
    trendSessions,
    comparisonSessions,
    selectedSessions,
    statsLoading,
    statsReady,
    statsDateMenu,
    periodDates,
    comparisonDates,
    overviewRangeLabel,
    trendRangeLabel,
    overviewPlannedMinutes,
    overviewActualSeconds,
    overviewPlanCount,
    overviewSessionCount,
    comparisonSubjects,
    plannedForDate,
    dailyActualSeconds,
    dailyActualTrend,
    completionLabel,
    plannedForPeriod,
    actualForPeriod,
    attachCharts,
    charts,
    resizeCharts,
    disposeCharts,
    loadStats,
    refreshStatsPage,
    leaveStatsPage,
    cancelStats,
    pickStatsDate,
  };
});
