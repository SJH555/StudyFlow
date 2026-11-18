import { onMounted, onUnmounted } from "vue";
import { System, Events } from "@wailsio/runtime";
import { preventDataCopy } from "./interactions/dataProtection";
import { useWindowFrame } from "./window/useWindowFrame";
import { usePlanDraft } from "./plans/usePlanDraft";
import { useAppStore } from "../stores/app";
import { useSettingsStore } from "../stores/settings";
import { useSessionsStore } from "../stores/sessions";
import { useActivityStore } from "../stores/activity";
import { useReviewStore } from "../stores/review";
import { useStatsStore } from "../stores/stats";

// 应用生命周期编排：挂载时启动初始化并注册全局事件/定时器，卸载时统一清理。
// 领域状态从各 store 读取；启动加载与对话框键盘处理由组合根（App.vue）注入。
type LifecycleDeps = {
  load: () => Promise<void>;
  handleDialogKeydown: (event: KeyboardEvent) => void;
};

export function useLifecycle(deps: LifecycleDeps) {
  const { load, handleDialogKeydown } = deps;
  const handleAppKeydown = (event: KeyboardEvent) => {
    if (event.key === "Tab") {
      event.preventDefault();
      event.stopImmediatePropagation();
      return;
    }
    handleDialogKeydown(event);
  };

  const appStore = useAppStore();
  const settingsStore = useSettingsStore();
  const sessionsStore = useSessionsStore();
  const activityStore = useActivityStore();
  const reviewStore = useReviewStore();
  const statsStore = useStatsStore();
  const { windowsTitlebar, windowsMaximised, syncWindowMaximised } =
    useWindowFrame(appStore.setError);
  const { closePlanNoteOutside } = usePlanDraft();

  let tick = 0;
  const refreshClock = () => appStore.tick();
  const refreshVisibleClock = () => {
    if (document.visibilityState === "visible") refreshClock();
  };

  onMounted(() => {
    System.Environment()
      .then((info) => {
        windowsTitlebar.value =
          windowsTitlebar.value || info.OS.toLowerCase() === "windows";
        syncWindowMaximised();
      })
      .catch(() => {});
    load();
    Events.On("daily-review-due", () => reviewStore.checkDailyReview());
    Events.On("common:WindowFocus", () => reviewStore.checkDailyReview());
    Events.On("common:WindowFocus", refreshClock);
    window.addEventListener("focus", refreshClock);
    document.addEventListener("visibilitychange", refreshVisibleClock);
    window.addEventListener("focus", reviewStore.checkDailyReview);
    Events.On("common:WindowMaximise", () => (windowsMaximised.value = true));
    Events.On(
      "common:WindowUnMaximise",
      () => (windowsMaximised.value = false),
    );
    Events.On("plan-reminder", (event: any) => {
      try {
        const alert = JSON.parse(event?.data || "{}") as {
          ended?: { name: string; saved: boolean };
          started?: { name: string };
        };
        if (!sessionsStore.showTaskReminder)
          sessionsStore.taskReminderEnded = null;
        if (alert.ended) sessionsStore.taskReminderEnded = alert.ended;
        appStore.tick();
        if (appStore.focusMode) appStore.exitFocus();
        sessionsStore.showTaskReminder = true;
        if (alert.ended)
          sessionsStore
            .refreshToday()
            .then(() => {
              if (appStore.page === "stats") return statsStore.refreshStatsPage();
            })
            .catch(appStore.setError);
      } catch (e) {
        appStore.setError(e);
      }
    });
    Events.On("activity-reminder", () =>
      activityStore.beginActivityReminder(),
    );
    Events.On("activity-reminder-state", (event: any) => {
      const enabled = event?.data === "1";
      activityStore.activityEnabled = enabled;
      settingsStore.settings.activityReminderEnabled = enabled ? "1" : "0";
    });
    Events.On("break-reminder", async (event: any) => {
      sessionsStore.breakReminderSubject = event?.data || "当前计划";
      try {
        await sessionsStore.refreshToday();
      } catch (e) {
        appStore.setError(e);
      }
      sessionsStore.showBreakReminder = true;
    });
    tick = window.setInterval(() => appStore.tick(), 1000);
    window.addEventListener("resize", statsStore.resizeCharts);
    document.addEventListener("copy", preventDataCopy);
    document.addEventListener("pointerdown", closePlanNoteOutside);
    document.addEventListener("keydown", handleAppKeydown, true);
  });

  onUnmounted(() => {
    clearInterval(tick);
    window.removeEventListener("resize", statsStore.resizeCharts);
    window.removeEventListener("focus", reviewStore.checkDailyReview);
    window.removeEventListener("focus", refreshClock);
    document.removeEventListener("visibilitychange", refreshVisibleClock);
    document.removeEventListener("copy", preventDataCopy);
    document.removeEventListener("pointerdown", closePlanNoteOutside);
    document.removeEventListener("keydown", handleAppKeydown, true);
    document.body.classList.remove("rest-orb-dragging");
    statsStore.disposeCharts();
  });
}
