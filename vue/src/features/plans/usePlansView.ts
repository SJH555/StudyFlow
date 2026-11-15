import { computed } from "vue";
import { usePlansStore } from "../../stores/plans";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlanDraft } from "./usePlanDraft";
import { plannedStudyMinutes } from "../../shared/format";

// 计划页视图派生：项目筛选列表/选项与今日学习分配（饼图与日程摘要）。
export function usePlansView() {
  const plansStore = usePlansStore();
  const subjectsStore = useSubjectsStore();
  const draft = usePlanDraft();

  // 按项目筛选后的列表。
  const managedPlans = computed(() =>
    plansStore.plans.filter(
      (p) =>
        !draft.planProjectFilter.value ||
        p.subjectId === draft.planProjectFilter.value,
    ),
  );

  const planFilterOptions = computed(() => [
    { title: "全部项目", value: 0 },
    ...subjectsStore.allSubjects
      .filter((item) => !item.deleted)
      .map((item) => ({
        title: `${item.name}（${plansStore.plans.filter((plan) => plan.subjectId === item.id).length} 段）`,
        value: item.id,
      })),
  ]);

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
    planFilterOptions,
    todayPlannedMinutes,
    planBreakdown,
    planPieSegments,
    todayPlanSummary,
  };
}
