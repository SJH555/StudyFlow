import { computed, nextTick, ref } from "vue";
import { useAppStore } from "../../stores/app";
import { usePlansStore } from "../../stores/plans";
import { useSessionsStore } from "../../stores/sessions";
import { plannedStudyMinutes } from "../../shared/format";

// 今日页视图状态：计划列表定位、日程弹窗和计划时长派生。
export function useTodayView() {
  const appStore = useAppStore();
  const plansStore = usePlansStore();
  const sessionsStore = useSessionsStore();

  // 今日计划列表 DOM 引用与日程展开。
  const todayPlanListEl = ref<HTMLElement | null>(null);
  const showTodaySchedule = ref(false);

  // 计划总时长（所选日期当天，今日页上即今日）。
  const plannedMinutes = computed(() =>
    plansStore.currentPlans.reduce((n, p) => n + plannedStudyMinutes(p), 0),
  );

  // 尚未学习或已经结束本次学习时，都提示当前计划还剩多久。
  const unstartedPlanRemainingSeconds = computed(() => {
    const plan = plansStore.currentPlan;
    if (!plan || sessionsStore.active) {
      return null;
    }

    const scheduledStart = new Date(appStore.now);
    const [hour, minute] = plan.startTime.split(":").map(Number);
    scheduledStart.setHours(hour, minute, 0, 0);
    if (scheduledStart.getTime() > appStore.now) {
      scheduledStart.setDate(scheduledStart.getDate() - 1);
    }
    const scheduledEnd = scheduledStart.getTime() + plan.duration * 60_000;
    return Math.max(0, Math.floor((scheduledEnd - appStore.now) / 1000));
  });

  // 时段空档定位到下一段计划，让列表中暂时不可见的计划也能找到。
  async function locateCurrentPlan() {
    const current = plansStore.todayPlans.find(
      (plan) => plan.id === plansStore.currentPlan?.id,
    );
    const now = new Date(appStore.now);
    const currentClock = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
    const next = plansStore.todayPlans.find(
      (plan) => plan.startTime > currentClock,
    );
    const target = current || next;
    if (!target) {
      appStore.notifyTop("今天没有待进行的计划时段");
      return;
    }
    await nextTick();
    const list = todayPlanListEl.value;
    const row = list?.querySelector<HTMLElement>(`[data-plan-id="${target.id}"]`);
    if (!list || !row) return;
    const listRect = list.getBoundingClientRect(),
      rowRect = row.getBoundingClientRect();
    list.scrollTo({
      top:
        list.scrollTop +
        rowRect.top -
        listRect.top -
        (list.clientHeight - row.clientHeight) / 2,
      behavior: "smooth",
    });
    if (!current) appStore.notifyTop("当前处于计划空档，已定位到下一段计划");
  }

  return {
    todayPlanListEl,
    showTodaySchedule,
    plannedMinutes,
    unstartedPlanRemainingSeconds,
    locateCurrentPlan,
  };
}
