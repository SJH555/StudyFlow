import { watch } from "vue";
import { useAppStore } from "../stores/app";
import { useSettingsStore } from "../stores/settings";
import { useSubjectsStore } from "../stores/subjects";
import { usePlansStore } from "../stores/plans";
import { useSessionsStore } from "../stores/sessions";
import { useActivityStore } from "../stores/activity";
import { useReviewStore } from "../stores/review";
import { useGoalsStore } from "../stores/goals";
import { useStatsStore } from "../stores/stats";

// 启动加载与页面切换协调（根部单次调用）：并行拉取各领域数据，
// 数据写入经各 store 的公开能力完成，并随页面/日期切换触发对应刷新。
export function useBootstrap() {
  const appStore = useAppStore();
  const settingsStore = useSettingsStore();
  const subjectsStore = useSubjectsStore();
  const plansStore = usePlansStore();
  const sessionsStore = useSessionsStore();
  const activityStore = useActivityStore();
  const reviewStore = useReviewStore();
  const goalsStore = useGoalsStore();
  const statsStore = useStatsStore();

  // 启动时加载全部核心数据并回填设置表单，完成后检查每日复习。
  async function load() {
    try {
      await Promise.all([
        settingsStore.loadSettings(),
        subjectsStore.fetchAll(),
        plansStore.fetch(),
        sessionsStore.fetchForDate(appStore.selectedDate),
        sessionsStore.refreshToday(),
        activityStore.fetchForDate(appStore.selectedDate),
        goalsStore.fetch(),
      ]);
      await settingsStore.loadDataDirectory();
      activityStore.applySettings();
      reviewStore.applySettings();
    } catch (e) {
      appStore.setError(e);
    } finally {
      appStore.setLoading(false);
      await reviewStore.checkDailyReview();
    }
  }

  // 切换页面 / 日期时按需刷新对应数据。
  watch(
    () => appStore.page,
    async (p) => {
    try {
      if (p !== "stats") statsStore.leaveStatsPage();
      if (p === "today") {
        if (appStore.selectedDate !== appStore.todayKey)
          appStore.selectedDate = appStore.todayKey;
        await sessionsStore.refreshToday();
      }
      if (p === "subjects" || p === "plans")
        await subjectsStore.refreshProjectAndPlan();
      if (p === "stats") await statsStore.refreshStatsPage();
      if (p === "summary") await reviewStore.refreshReviewHistory();
    } catch (e) {
      appStore.setError(e);
    }
  });

  watch(
    () => appStore.selectedDate,
    async (d) => {
    try {
      if (appStore.page === "stats") await statsStore.refreshStatsPage();
      else
        await Promise.all([
          sessionsStore.fetchForDate(d),
          activityStore.fetchForDate(d),
        ]);
    } catch (e) {
      appStore.setError(e);
    }
  });

  // 重置表/停止学习后重载全部数据。
  appStore.setReloadAll(load);

  return { load };
}
