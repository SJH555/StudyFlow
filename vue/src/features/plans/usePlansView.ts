import { computed, type Ref } from "vue";
import { usePlansStore } from "../../stores/plans";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlanDraft } from "./usePlanDraft";
import { plannedStudyMinutes } from "../../shared/format";

// 计划页视图派生：项目筛选列表/选项与今日学习分配（饼图与日程摘要）。
export type PlanTab = "active" | "stopped";

export function usePlansView(tab: Ref<PlanTab>) {
  const plansStore = usePlansStore();
  const subjectsStore = useSubjectsStore();
  const draft = usePlanDraft();

  const activePlans = computed(() => plansStore.managedPlans.filter((p) => p.managedActive));
  const stoppedPlans = computed(() => plansStore.managedPlans.filter((p) => !p.managedActive));

  const tabPlans = computed(() => tab.value === "stopped" ? stoppedPlans.value : activePlans.value);

  const managedPlans = computed(() =>
    tabPlans.value.filter(
      (p) => !draft.planProjectFilter.value || p.subjectId === draft.planProjectFilter.value,
    ),
  );

  const planFilterOptions = computed(() => {
    const projects = new Map<number, string>();
    for (const plan of tabPlans.value) {
      if (!projects.has(plan.subjectId)) projects.set(plan.subjectId, plan.subject);
    }
    for (const subject of subjectsStore.managedSubjects) {
      if (projects.has(subject.id) && !subject.deleted) projects.set(subject.id, subject.name);
    }
    return [
      { title: "全部项目", value: 0 },
      ...Array.from(projects, ([value, title]) => ({ title, value })),
    ];
  });

  // 今日计划按科目聚合（饼图）与今日日程摘要（起始/结束/学习/休息/空档）。
  const planBreakdown = computed(() => {
    const groups = new Map<
      number,
      { id: number; name: string; color: string; minutes: number }
    >();
    for (const p of plansStore.todayPlans) {
      const item = groups.get(p.subjectId) || {
        id: p.subjectId,
        name: p.subject,
        color: p.color,
        minutes: 0,
      };
      item.minutes += plannedStudyMinutes(p);
      groups.set(p.subjectId, item);
    }
    return [...groups.values()].sort((a, b) => b.minutes - a.minutes);
  });

  const todayPlannedMinutes = computed(() =>
    plansStore.todayPlans.reduce((n, p) => n + plannedStudyMinutes(p), 0),
  );

  const planPieSegments = computed(() => {
    const circumference = 2 * Math.PI * 40;
    let offset = 0;
    return planBreakdown.value.map((item) => {
      const length =
        (item.minutes / (todayPlannedMinutes.value || 1)) * circumference;
      const segment = { ...item, length, offset };
      offset += length;
      return segment;
    });
  });

  const todayPlanSummary = computed(() => {
    const intervals = plansStore.todayPlans
      .map((p) => {
        const [hour, minute] = p.startTime.split(":").map(Number);
        const start = hour * 60 + minute;
        return {
          start,
          end: start + p.duration,
          breakMinutes: p.breakMinutes || 0,
        };
      })
      .sort((a, b) => a.start - b.start);
    if (!intervals.length) return null;
    let lastEnd = intervals[0].end,
      gapMinutes = 0;
    for (const interval of intervals.slice(1)) {
      gapMinutes += Math.max(0, interval.start - lastEnd);
      lastEnd = Math.max(lastEnd, interval.end);
    }
    const breakMinutes = intervals.reduce(
      (total, interval) => total + interval.breakMinutes,
      0,
    );
    return {
      firstStart: intervals[0].start,
      lastEnd,
      studyMinutes: todayPlannedMinutes.value,
      breakMinutes,
      gapMinutes,
      totalRestMinutes: breakMinutes + gapMinutes,
    };
  });

  return {
    managedPlans,
    activePlans,
    stoppedPlans,
    planFilterOptions,
    todayPlannedMinutes,
    planBreakdown,
    planPieSegments,
    todayPlanSummary,
  };
}
