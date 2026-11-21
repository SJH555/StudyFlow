import { nextTick, watch } from "vue";
import { api } from "../api";
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
    const started = performance.now();
    try {
      await Promise.all([
        settingsStore.loadSettings(),
        subjectsStore.fetchAll(),
        plansStore.fetch(),
        sessionsStore.loadInitial(),
        goalsStore.fetch(),
      ]);

      activityStore.applySettings();
      reviewStore.applySettings();
    } catch (e) {
      appStore.setError(e);
    } finally {
      appStore.setLoading(false);
      performance.clearMeasures("studyflow:home-data");
      performance.measure("studyflow:home-data", {
        start: started,
        end: performance.now(),
      });
      await nextTick();
      // 复盘可能扫描历史日期，先完成它再释放维护任务，避免备份抢占同一连接。
      await reviewStore.checkDailyReview();
      const backgroundResults = await Promise.allSettled([
        activityStore.fetchForDate(appStore.selectedDate),
        settingsStore.loadDataDirectory(),
      ]);
      for (const result of backgroundResults) {
        if (result.status === "rejected") appStore.setError(result.reason);
      }
      await api.homeReady().catch(appStore.setError);
    }
  }

  // 只在本地日期变化时跟进，保留正在查看的历史日期。
  watch(
    () => appStore.todayKey,
    (today, previousToday) => {
      if (appStore.selectedDate === previousToday) {
        appStore.selectedDate = today;
      }
    },
  );

  // 一次页面或日期变更只刷新当前页面，切入活动页时再加载对应日期。
  watch(
    [() => appStore.page, () => appStore.selectedDate],
    async ([page, date], [previousPage]) => {
      const pageChanged = page !== previousPage;
      try {
        if (previousPage === "stats" && pageChanged) statsStore.leaveStatsPage();
        if (page === "today") {
          if (pageChanged && date !== appStore.todayKey) {
            appStore.selectedDate = appStore.todayKey;
            return;
          }
          await sessionsStore.refreshToday();
        } else if (page === "stats") {
          await statsStore.refreshStatsPage();
        } else if (page === "activity") {
          await activityStore.fetchForDate(date);
        } else if (pageChanged && (page === "subjects" || page === "plans")) {
          await subjectsStore.refreshProjectAndPlan();
        } else if (pageChanged && page === "summary") {
          await reviewStore.refreshReviewHistory();
        }
      } catch (e) {
        appStore.setError(e);
      }
    },
  );

  // 重置表后重载全部数据。
  appStore.setReloadAll(load);

  return { load };
}
