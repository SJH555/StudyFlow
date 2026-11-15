import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { acknowledgeReminder, recordClientError } from "../api";
import { dateKey } from "../shared/format";

// 应用级共享状态：页面路由、共享时钟与日期、启动加载、全局错误与提示、专注模式。
export const useAppStore = defineStore("app", () => {
  const page = ref("today");
  const now = ref(Date.now());
  const selectedDate = ref(dateKey(new Date()));
  const loading = ref(true);
  const error = ref("");
  const topNotice = ref("");
  const focusMode = ref(false);

  // 由共享时钟派生，避免维护两份日期状态。
  const todayKey = computed(() => dateKey(new Date(now.value)));

  function tick() {
    now.value = Date.now();
  }

  function setLoading(value: boolean) {
    loading.value = value;
  }

  function setError(e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
    recordClientError(error.value).catch(() => {});
    // 错误提示 5 秒后自动清除。
    setTimeout(() => (error.value = ""), 5000);
  }

  function notifyTop(message: string) {
    topNotice.value = message;
  }

  function clearTopNotice() {
    topNotice.value = "";
  }

  // 停止提醒提示音（任务提醒/活动提醒/休息提醒共用）。
  async function stopReminderSound() {
    try {
      await acknowledgeReminder();
    } catch (e) {
      setError(e);
    }
  }

  // 重载全部数据的启动编排，由组合根（useBootstrap）注入。
  let reloadAll: (() => Promise<void>) | null = null;
  function setReloadAll(reload: () => Promise<void>) {
    reloadAll = reload;
  }
  async function reload() {
    if (reloadAll) await reloadAll();
  }

  function enterFocus() {
    focusMode.value = true;
    document.documentElement.requestFullscreen?.().catch(() => {});
  }

  function exitFocus() {
    focusMode.value = false;
    if (document.fullscreenElement) document.exitFullscreen?.().catch(() => {});
  }

  return {
    page,
    now,
    selectedDate,
    loading,
    error,
    topNotice,
    focusMode,
    todayKey,
    tick,
    setLoading,
    setError,
    notifyTop,
    clearTopNotice,
    stopReminderSound,
    setReloadAll,
    reload,
    enterFocus,
    exitFocus,
  };
});
