import { nextTick, type ComputedRef, type Ref } from "vue";
import { storeToRefs } from "pinia";
import type { ECharts } from "echarts";
import { api, type Subject } from "../../api";
import { dateKey, fmt, formatDateChinese } from "../../shared/format";
import { useAppStore } from "../../stores/app";
import { useStatsStore } from "../../stores/stats";

type ExportDeps = {
  selectedDate: Ref<string>;
  loadStats: () => Promise<void>;
  getChart: () => ECharts | null;
  getProjectChart: () => ECharts | null;
  periodDates: ComputedRef<Date[]>;
  plannedForDate: (date: Date, subjectId?: number) => number;
  dailyActualSeconds: (date: Date) => number;
  comparisonSubjects: ComputedRef<Subject[]>;
  plannedForPeriod: (subjectId?: number) => number;
  actualForPeriod: (subjectId?: number) => number;
  error: Ref<string>;
  setError: (error: unknown) => void;
};

// 导出 HTML 报告：克隆当前统计/活动页面为静态 HTML，图表转为图片后保存文件。
export async function exportHtmlReport(
  kind: "study" | "activity",
  deps: ExportDeps,
) {
  const {
    selectedDate,
    loadStats,
    getChart,
    getProjectChart,
    periodDates,
    plannedForDate,
    dailyActualSeconds,
    comparisonSubjects,
    plannedForPeriod,
    actualForPeriod,
    error,
    setError,
  } = deps;
  const title = kind === "study" ? "学习统计" : "活动提醒";
  try {
    if (kind === "study") await loadStats();
    await nextTick();
    const source = document.querySelector<HTMLElement>(".content");
    if (!source) throw new Error("当前页面尚未加载完成");
    const copy = source.cloneNode(true) as HTMLElement;
    const heading = copy.querySelector(".heading-actions");
    if (heading)
      heading.innerHTML = `<span class="export-date">${formatDateChinese(selectedDate.value)}</span>`;
    if (kind === "study") {
      for (const [selector, instance] of [
        [".chart", getChart()],
        [".project-chart", getProjectChart()],
      ] as const) {
        const target = copy.querySelector<HTMLElement>(selector);
        const original = source.querySelector<HTMLElement>(selector);
        if (target && original && instance) {
          const image = document.createElement("img");
          image.src = instance.getDataURL({
            type: "png",
            pixelRatio: 2,
            backgroundColor: "#fff",
          });
          image.alt =
            selector === ".chart" ? "学习趋势图" : "学习项目计划与实际对比图";
          image.className = "export-chart-image";
          image.style.height = `${original.clientHeight}px`;
          target.replaceChildren(image);
        }
      }
      const appendTable = (
        selector: string,
        headers: string[],
        rows: string[][],
      ) => {
        const panel = copy.querySelector(selector);
        if (!panel) return;
        const table = document.createElement("table");
        table.className = "export-table";
        const head = document.createElement("tr");
        for (const label of headers) {
          const th = document.createElement("th");
          th.textContent = label;
          head.append(th);
        }
        table.append(head);
        for (const values of rows) {
          const tr = document.createElement("tr");
          for (const value of values) {
            const td = document.createElement("td");
            td.textContent = value;
            tr.append(td);
          }
          table.append(tr);
        }
        panel.append(table);
      };
      appendTable(
        ".trend-wide",
        ["日期", "计划学习", "实际学习"],
        periodDates.value.map((date) => [
          formatDateChinese(dateKey(date)),
          fmt(plannedForDate(date) * 60),
          fmt(dailyActualSeconds(date)),
        ]),
      );
      appendTable(
        ".project-compare-panel",
        ["学习项目", "计划时长", "实际时长"],
        comparisonSubjects.value.map((subject) => [
          subject.name,
          fmt(plannedForPeriod(subject.id) * 60),
          fmt(actualForPeriod(subject.id)),
        ]),
      );
    }
    const originals = [
      ...source.querySelectorAll<
        HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
      >("input,select,textarea"),
    ].filter((field) => !field.closest(".heading-actions"));
    const copies = copy.querySelectorAll<
      HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
    >("input,select,textarea");
    copies.forEach((field, index) => {
      const original = originals[index];
      const value =
        original instanceof HTMLSelectElement
          ? original.selectedOptions[0]?.textContent || original.value
          : original.value;
      const span = document.createElement("span");
      span.className = "export-field";
      span.textContent = value;
      field.replaceWith(span);
    });
    copy.querySelectorAll(".period-switch").forEach((group) => {
      const selected =
        group.querySelector(".selected")?.textContent?.trim() || "";
      const label = group.getAttribute("aria-label") || "范围";
      const span = document.createElement("span");
      span.className = "export-period";
      span.textContent = `${label}：${selected}`;
      group.replaceWith(span);
    });
    copy.querySelectorAll("button").forEach((button) => button.remove());
    const css = (
      await Promise.all(
        [...document.styleSheets].map(async (sheet) => {
          try {
            return [...sheet.cssRules].map((rule) => rule.cssText).join("\n");
          } catch {
            try {
              return sheet.href ? await (await fetch(sheet.href)).text() : "";
            } catch {
              return "";
            }
          }
        }),
      )
    ).join("\n");
    const exportCss = `${css}\nbody{min-width:0;background:#fff}.shell{min-height:0}.main{margin-left:0;min-height:0}.content{max-width:1160px;margin:0 auto;padding:32px 28px}.compare-scroll{max-height:none!important;overflow:visible!important}.export-chart-image{display:block;width:100%;object-fit:contain}.export-date,.export-period,.export-field{display:inline-block;padding:8px 12px;background:#eef1ff;border-radius:8px;color:#3445a5;font-size:13px}.export-period{white-space:nowrap}.export-table{width:100%;border-collapse:collapse;margin-top:20px;font-size:13px}.export-table th,.export-table td{padding:10px 12px;text-align:left;border-bottom:1px solid #e8ebf2}.export-table th{background:#eef1ff;color:#35406c}.activity-reminder-card{break-inside:avoid}@media print{body{-webkit-print-color-adjust:exact;print-color-adjust:exact}.content{padding:12px}.panel,.metric-card{break-inside:avoid}}`;
    const html = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${title} · ${selectedDate.value}</title><style>${exportCss}</style></head><body><div class="shell"><main class="main">${copy.outerHTML}</main></div></body></html>`;
    const path = await api.saveHtml(
      `${title}-${selectedDate.value}.html`,
      html,
    );
    if (path) {
      error.value = `已导出到：${path}`;
      setTimeout(() => (error.value = ""), 4500);
    }
  } catch (e) {
    setError(e);
  }
}

// 导出当前页面的 HTML 报告：统计页复用 stats store 的数据与页面注入的图表实例。
export async function exportHtml(kind: "study" | "activity") {
  const appStore = useAppStore();
  const statsStore = useStatsStore();
  const { selectedDate, error } = storeToRefs(appStore);
  const { periodDates, comparisonSubjects } = storeToRefs(statsStore);
  return exportHtmlReport(kind, {
    selectedDate,
    loadStats: statsStore.loadStats,
    getChart: () => statsStore.charts?.getChart() ?? null,
    getProjectChart: () => statsStore.charts?.getProjectChart() ?? null,
    periodDates,
    plannedForDate: statsStore.plannedForDate,
    dailyActualSeconds: statsStore.dailyActualSeconds,
    comparisonSubjects,
    plannedForPeriod: statsStore.plannedForPeriod,
    actualForPeriod: statsStore.actualForPeriod,
    error,
    setError: appStore.setError,
  });
}
