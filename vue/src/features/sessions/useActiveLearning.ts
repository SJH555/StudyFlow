import { computed } from "vue";
import { useAppStore } from "../../stores/app";
import { usePlansStore } from "../../stores/plans";
import { useSessionsStore } from "../../stores/sessions";
import { plannedStudyMinutes } from "../../shared/format";

// 进行中学习的派生视图：当前计划、倒计时、休息剩余与实际/可见记录。
// 供今日页、专注浮层与休息悬浮球共享；状态属于 sessions/plans/app store。
export function useActiveLearning() {
  const appStore = useAppStore();
  const plansStore = usePlansStore();
  const sessionsStore = useSessionsStore();

  // 当前活动会话对应的计划。
  const activePlan = computed(() =>
    plansStore.plans.find((p) => p.id === sessionsStore.active?.planId),
  );

  const actualSeconds = computed(() =>
    sessionsStore.sessions.reduce(
      (n, s) => n + sessionsStore.sessionSeconds(s),
      0,
    ),
  );

  // 600 秒即 10 分钟：不足 10 分钟的学习记录不展示（后端同样未计入）。
  const visibleSessions = computed(() =>
    sessionsStore.sessions.filter((s) => !s.endAt || s.duration >= 600),
  );

  // 结束时刻只随会话/计划变化解析，不随每秒时钟更新重复解析。
  const countdownEndAt = computed(() => {
    const session = sessionsStore.active;
    if (!session) return null;
    if (session.plannedEndAt) {
      const end = new Date(session.plannedEndAt).getTime();
      if (Number.isFinite(end)) return end;
    }
    // 兼容未保存结束边界的历史会话，以会话开始日期定位计划时段。
    const plan = activePlan.value;
    if (!plan) return null;
    const startedAt = new Date(session.startAt).getTime();
    const start = new Date(startedAt);
    const [hour, minute] = plan.startTime.split(":").map(Number);
    start.setHours(hour, minute, 0, 0);
    let end = start.getTime() + plan.duration * 60_000;
    if (start.getTime() > startedAt) {
      const previous = new Date(start);
      previous.setDate(previous.getDate() - 1);
      const previousEnd = previous.getTime() + plan.duration * 60_000;
      if (previousEnd > startedAt) end = previousEnd;
    }
    return end;
  });

  const heroBreakMinutes = computed(
    () => sessionsStore.active?.planVersionId
      ? sessionsStore.active.breakMinutes ?? 0
      : (activePlan.value || plansStore.currentPlan)?.breakMinutes || 0,
  );

  const countdownTotal = computed(() => {
    const session = sessionsStore.active;
    if (session && countdownEndAt.value !== null) {
      return Math.max(
        1,
        (countdownEndAt.value - new Date(session.startAt).getTime()) / 1000,
      );
    }
    const plan = activePlan.value || plansStore.currentPlan;
    return Math.max(1, plan ? plannedStudyMinutes(plan) : 60) * 60;
  });

  // 距离计划结束的时间包含暂停时间，实际学习时长仍单独累计。
  const countdownLeft = computed(() => {
    if (countdownEndAt.value !== null) {
      return Math.max(0, Math.floor((countdownEndAt.value - appStore.now) / 1000));
    }
    return Math.max(
      0,
      countdownTotal.value -
        (sessionsStore.active
          ? sessionsStore.sessionSeconds(sessionsStore.active)
          : 0),
    );
  });

  const countdownPercent = computed(() =>
    Math.min(
      100,
      Math.max(0, (1 - countdownLeft.value / countdownTotal.value) * 100),
    ),
  );

  // 休息中的计划与剩余休息秒数。
  const activeBreakPlan = computed(() => {
    const session = sessionsStore.active;
    if (!session?.pausedAt || !session.breakStartedAt || !heroBreakMinutes.value) return null;
    return { breakMinutes: heroBreakMinutes.value };
  });

  const breakLeftSeconds = computed(() => {
    const session = sessionsStore.active;
    if (!activeBreakPlan.value || !session?.breakStartedAt) return 0;
    const remaining = activeBreakPlan.value.breakMinutes * 60 -
      (session.breakUsedSeconds || 0) -
      Math.floor((appStore.now - new Date(session.breakStartedAt).getTime()) / 1000);
    return Math.max(0, countdownEndAt.value === null ? remaining : Math.min(remaining, countdownLeft.value));
  });

  return {
    activePlan,
    actualSeconds,
    visibleSessions,
    heroBreakMinutes,
    countdownLeft,
    countdownPercent,
    activeBreakPlan,
    breakLeftSeconds,
  };
}
