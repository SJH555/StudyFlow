import { ref, type ComputedRef, type Ref } from "vue";
import * as echarts from "echarts";
import { useTheme } from "vuetify";
import type { Subject } from "../../api";
import { dateKey, fmt, formatDateChinese } from "../../shared/format";
import { chartSeriesColors, chartTheme } from "../../shared/chartTheme";

// 图表渲染：用 ECharts 渲染学习趋势图与项目对比图，并提供导出所需的实例引用。
type ChartInputs = {
  periodDates: ComputedRef<Date[]>;
  statsPeriod: Ref<"week" | "month">;
  comparisonSubjects: ComputedRef<Subject[]>;
  plannedForDate: (date: Date, subjectId?: number) => number;
  dailyActualSeconds: (date: Date) => number;
  dailyActualTrend: (date: Date) => number | null;
  completionLabel: (plannedSeconds: number, actualSeconds: number) => string;
  plannedForPeriod: (subjectId?: number) => number;
  actualForPeriod: (subjectId?: number) => number;
};

export function useCharts({
  periodDates,
  statsPeriod,
  comparisonSubjects,
  plannedForDate,
  dailyActualSeconds,
  dailyActualTrend,
  completionLabel,
  plannedForPeriod,
  actualForPeriod,
}: ChartInputs) {
  const theme = useTheme();
  const chartEl = ref<HTMLElement | null>(null);
  const projectChartEl = ref<HTMLElement | null>(null);
  let chart: echarts.ECharts | null = null;
  let projectChart: echarts.ECharts | null = null;

  function seriesColors() {
    // 计划柱色直接读取上方卡片的最终边框色，避免主题作用域导致色差。
    const planCard = document.querySelector<HTMLElement>(
      ".stats-main .summary-card--tall.summary-card--primary",
    );
    const borderColor = planCard
      ? getComputedStyle(planCard).borderTopColor
      : undefined;
    return chartSeriesColors(theme.current.value.colors, borderColor);
  }

  function renderChart() {
    if (!chartEl.value) return;
    chart?.dispose();
    chart = echarts.init(chartEl.value);
    const colors = seriesColors();
    chart.setOption({
      color: [colors.plan, colors.actual, colors.trend],
      tooltip: {
        trigger: "axis",
        formatter: (items: any[]) => {
          const date = periodDates.value[items[0]?.dataIndex];
          if (!date) return "";
          const planned = plannedForDate(date),
            actual = dailyActualSeconds(date);
          return `<b>${formatDateChinese(dateKey(date))}</b><br>计划学习：${fmt(planned * 60)}<br>实际学习：${fmt(actual)}<br>完成度：${completionLabel(planned * 60, actual)}`;
        },
      },
      legend: { bottom: 0, data: ["计划学习", "实际学习", "实际学习趋势"] },
      grid: { left: 40, right: 18, top: 24, bottom: 52 },
      xAxis: {
        type: "category",
        data: periodDates.value.map((d) =>
          statsPeriod.value === "week"
            ? d.toLocaleDateString("zh-CN", { weekday: "short" })
            : String(d.getDate()),
        ),
        axisLine: { lineStyle: { color: chartTheme.axisLine } },
        axisLabel: {
          color: chartTheme.axisLabel,
          interval: statsPeriod.value === "week" ? 0 : "auto",
        },
      },
      yAxis: {
        type: "value",
        min: 0,
        splitLine: { lineStyle: { color: chartTheme.splitLine } },
        axisLabel: { color: chartTheme.axisLabel },
      },
      series: [
        {
          name: "计划学习",
          type: "bar",
          barMaxWidth: 16,
          itemStyle: { borderRadius: [5, 5, 0, 0] },
          data: periodDates.value.map((d) => plannedForDate(d) / 60),
        },
        {
          name: "实际学习",
          type: "bar",
          barMaxWidth: 16,
          itemStyle: { borderRadius: [5, 5, 0, 0] },
          data: periodDates.value.map((d) => dailyActualSeconds(d) / 3600),
        },
        {
          name: "实际学习趋势",
          type: "line",
          connectNulls: false,
          symbol: "circle",
          symbolSize: 7,
          lineStyle: { width: 3 },
          data: periodDates.value.map((d) => dailyActualTrend(d)),
        },
      ],
    });
  }

  function resizeCharts() {
    chart?.resize();
    projectChart?.resize();
  }

  function renderProjectChart() {
    if (!projectChartEl.value) {
      projectChart?.dispose();
      projectChart = null;
      return;
    }
    projectChart?.dispose();
    projectChart = echarts.init(projectChartEl.value);
    const rows = comparisonSubjects.value;
    const colors = seriesColors();
    projectChart.setOption({
      color: [colors.plan, colors.actual],
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        formatter: (items: any[]) => {
          const index = items[0]?.dataIndex,
            subject = rows[index];
          if (!subject) return "";
          const planned = plannedForPeriod(subject.id) * 60,
            actual = actualForPeriod(subject.id);
          return `<b>${subject.name}</b><br>${items.map((p) => `${p.marker}${p.seriesName}：${fmt(Math.round(Number(p.value) * 3600))}`).join("<br>")}<br>完成度：${completionLabel(planned, actual)}`;
        },
      },
      legend: { bottom: 0, data: ["计划时长", "实际时长"] },
      grid: { left: 125, right: 95, top: 18, bottom: 48 },
      xAxis: {
        type: "value",
        min: 0,
        axisLabel: {
          formatter: (value: number) => `${value}h`,
          color: chartTheme.projectAxisLabel,
        },
        splitLine: { lineStyle: { color: chartTheme.projectSplitLine } },
      },
      yAxis: {
        type: "category",
        inverse: true,
        data: rows.map((s) => s.name),
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: chartTheme.projectCategoryLabel, fontSize: 13 },
      },
      series: [
        {
          name: "计划时长",
          type: "bar",
          barMaxWidth: 20,
          itemStyle: { borderRadius: [0, 5, 5, 0] },
          label: {
            show: true,
            position: "right",
            formatter: (p: any) => fmt(Math.round(Number(p.value) * 3600)),
            color: chartTheme.projectValueLabel,
            fontSize: 12,
          },
          data: rows.map((s) => plannedForPeriod(s.id) / 60),
        },
        {
          name: "实际时长",
          type: "bar",
          barMaxWidth: 20,
          itemStyle: { borderRadius: [0, 5, 5, 0] },
          label: {
            show: true,
            position: "right",
            formatter: (p: any) => fmt(Math.round(Number(p.value) * 3600)),
            color: chartTheme.projectValueLabel,
            fontSize: 12,
          },
          data: rows.map((s) => actualForPeriod(s.id) / 3600),
        },
      ],
    });
  }

  function disposeCharts() {
    chart?.dispose();
    chart = null;
    projectChart?.dispose();
    projectChart = null;
  }
  function getChart() {
    return chart;
  }
  function getProjectChart() {
    return projectChart;
  }
  return {
    chartEl,
    projectChartEl,
    renderChart,
    renderProjectChart,
    resizeCharts,
    disposeCharts,
    getChart,
    getProjectChart,
  };
}
