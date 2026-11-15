import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { api, type Activity } from "../api";
import { parseDateKey } from "../shared/format";
import { useAppStore } from "./app";
import { useSettingsStore } from "./settings";

// 学习活动：活动记录与提醒的所有者，负责活动记录、提醒弹窗交互与提醒时间/间隔设置。
export const useActivityStore = defineStore("activity", () => {
  const appStore = useAppStore();
  const settingsStore = useSettingsStore();

  const activities = ref<Activity[]>([]);
  const activityModalWasOpen = ref(false);
  const activityDraftBeforeReminder = ref<string | null>(null);
  const activityText = ref("");
  const reminderActive = ref(false);
  const activityEnabled = ref(false);
  const activitySettingsSaving = ref(false);
  const activityIntervalInput = ref("30");
  const activityStartHour = ref("09");
  const activityStartMinute = ref("00");
  const activitySkipRest = ref(false);
  const activityDateMenu = ref(false);
  const showActivity = ref(false);

  const activityIntervalNumber = computed({
    get: () => Number(activityIntervalInput.value) || null,
    set: (value: number | null) => {
      activityIntervalInput.value = value == null ? "" : String(value);
    },
  });

  const activitySettingsDirty = computed(
    () =>
      `${activityStartHour.value}:${activityStartMinute.value}` !==
        (settingsStore.settings.activityStartTime || "09:00") ||
      activityIntervalInput.value.trim() !==
        (settingsStore.settings.activityInterval || "30") ||
      activitySkipRest.value !==
        (settingsStore.settings.activitySkipRest === "1"),
  );

  // 启动加载设置后回填表单与开关。
  function applySettings() {
    activityEnabled.value =
      settingsStore.settings.activityReminderEnabled === "1";
    activityIntervalInput.value =
      settingsStore.settings.activityInterval || "30";
    activitySkipRest.value = settingsStore.settings.activitySkipRest === "1";
    const reminderStart = settingsStore.settings.activityStartTime || "09:00";
    activityStartHour.value = reminderStart.slice(0, 2);
    activityStartMinute.value = reminderStart.slice(3, 5);
  }

  // 按日期拉取活动记录（启动加载/日期切换时由编排方调用）。
  async function fetchForDate(d: string) {
    activities.value = await api.activities(d);
    return activities.value;
  }

  // 提醒打断时暂存正在编辑的内容，结束后再恢复原状态。
  function beginActivityReminder() {
    if (reminderActive.value) return;
    activityModalWasOpen.value = showActivity.value;
    if (showActivity.value)
      activityDraftBeforeReminder.value = activityText.value;
    activityText.value = "";
    reminderActive.value = true;
  }

  function finishActivityReminder() {
    reminderActive.value = false;
    showActivity.value = activityModalWasOpen.value;
    activityText.value = activityDraftBeforeReminder.value || "";
    activityModalWasOpen.value = false;
    activityDraftBeforeReminder.value = null;
  }

  async function dismissActivityReminder() {
    await appStore.stopReminderSound();
    finishActivityReminder();
  }

  function openActivity() {
    reminderActive.value = false;
    activityText.value = "";
    showActivity.value = true;
  }

  async function addActivity() {
    if (!activityText.value.trim()) {
      appStore.notifyTop("请填写当前活动");
      return;
    }
    try {
      const fromReminder = reminderActive.value;
      if (fromReminder) await appStore.stopReminderSound();
      await api.addActivity(activityText.value.trim());
      if (fromReminder) finishActivityReminder();
      else {
        activityText.value = "";
        showActivity.value = false;
      }
      activities.value = await api.activities(appStore.selectedDate);
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function saveActivitySchedule(enable = activityEnabled.value, fromToggle = false) {
    if (activitySettingsSaving.value) return;
    const interval = Number(activityIntervalInput.value.trim());
    // 1440 为一天分钟数，作为提醒间隔上限。
    if (
      !/^\d+$/.test(activityIntervalInput.value.trim()) ||
      !Number.isSafeInteger(interval) ||
      interval < 1 ||
      interval > 1440
    ) {
      appStore.notifyTop("提醒间隔请输入 1 到 1440 之间的整数分钟");
      return;
    }
    const start = activityStartHour.value + ":" + activityStartMinute.value;
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(start)) {
      appStore.notifyTop("请选择有效的提醒开始时间");
      return;
    }
    activitySettingsSaving.value = true;
    const saved = {
      activityStartTime: start,
      activityInterval: String(interval),
      activityReminderEnabled: enable ? "1" : "0",
      activitySkipRest: activitySkipRest.value ? "1" : "0",
    };
    try {
      await api.saveSettings(saved);
      Object.assign(settingsStore.settings, saved);
      activityEnabled.value = enable;
      activityIntervalInput.value = String(interval);
      appStore.notifyTop(fromToggle ? "活动提醒已开启" : "活动提醒设置已保存");
    } catch (e) {
      appStore.setError(e);
    } finally {
      activitySettingsSaving.value = false;
    }
  }

  async function toggleActivityReminder() {
    if (activitySettingsSaving.value) return;
    if (activityEnabled.value) {
      try {
        await api.saveSettings({ activityReminderEnabled: "0" });
        settingsStore.settings.activityReminderEnabled = "0";
        activityEnabled.value = false;
        appStore.notifyTop("活动提醒已关闭");
      } catch (e) {
        appStore.setError(e);
      }
    } else await saveActivitySchedule(true, true);
  }

  function pickActivityDate(value: unknown) {
    const day = parseDateKey(value);
    if (day) {
      appStore.selectedDate = day;
      activityDateMenu.value = false;
    }
  }

  return {
    activities,
    activityModalWasOpen,
    activityDraftBeforeReminder,
    activityText,
    reminderActive,
    activityEnabled,
    activitySettingsSaving,
    activityIntervalInput,
    activityStartHour,
    activityStartMinute,
    activitySkipRest,
    activityDateMenu,
    showActivity,
    activityIntervalNumber,
    activitySettingsDirty,
    applySettings,
    fetchForDate,
    beginActivityReminder,
    finishActivityReminder,
    dismissActivityReminder,
    openActivity,
    addActivity,
    saveActivitySchedule,
    toggleActivityReminder,
    pickActivityDate,
  };
});
