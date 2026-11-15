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

  // 倒计时目标：优先锚定到计划的安排时段（提前/晚开始都以计划结束时刻为准，
  // 计时在时段结束自动停止），无计划边界时回退为计划学习分钟数。
  const countdownTargetMinutes = computed(() => {
    const session = sessionsStore.active;
    if (session?.plannedEndAt) {
      const minutes = Math.round(
        (new Date(session.plannedEndAt).getTime() -
          new Date(session.startAt).getTime()) /
          60000,
      );
      if (minutes > 0) return minutes;
    }
    const plan = activePlan.value || plansStore.currentPlan;
    return plan ? plannedStudyMinutes(plan) : 60;
  });

  const heroBreakMinutes = computed(
    () => (activePlan.value || plansStore.currentPlan)?.breakMinutes || 0,
  );

  const countdownTotal = computed(
    () => Math.max(1, countdownTargetMinutes.value) * 60,
  );

  const countdownLeft = computed(() =>
    Math.max(
      0,
      countdownTotal.value -
        (sessionsStore.active
          ? sessionsStore.sessionSeconds(sessionsStore.active)
          : 0),
    ),
  );

  const countdownPercent = computed(() =>
    Math.min(
      100,
      Math.max(0, (1 - countdownLeft.value / countdownTotal.value) * 100),
    ),
  );

  // 休息中的计划与剩余休息秒数。
  const activeBreakPlan = computed(() =>
    sessionsStore.active?.pausedAt &&
    sessionsStore.active.breakStartedAt &&
    activePlan.value?.breakMinutes
      ? activePlan.value
      : null,
  );

  const breakLeftSeconds = computed(() => {
    const session = sessionsStore.active;
    if (!activeBreakPlan.value || !session?.breakStartedAt) return 0;
    return Math.max(
      0,
      activeBreakPlan.value.breakMinutes * 60 -
        (session.breakUsedSeconds || 0) -
        Math.floor(
          (appStore.now - new Date(session.breakStartedAt).getTime()) / 1000,
        ),
    );
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
