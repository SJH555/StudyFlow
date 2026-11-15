import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  api,
  errorLogs,
  readErrorLog,
  recordClientError,
  resetTables,
  resetTable,
  type ResetTable,
  type ErrorLogFile,
} from "../api";
import { useAppStore } from "./app";
import {
  navigationWidths,
  parseNavigationWidth,
  type NavigationLevel,
} from "../shared/navigationWidth";

// 应用设置：配置数据的所有者，负责常规项（自启动、铃声、窗口行为）、数据目录、
// 自动备份、日志查看与数据重置。
export const useSettingsStore = defineStore("settings", () => {
  const appStore = useAppStore();

  const settings = ref<Record<string, string>>({ activityInterval: "30" }); // 活动提醒默认间隔 30 分钟
  const settingsTab = ref<"general" | "storage" | "logs" | "reset">("general");
  const resetTableList = ref<ResetTable[]>([]);
  const resetTarget = ref<ResetTable | null>(null);
  const logFiles = ref<ErrorLogFile[]>([]);
  const logDirectory = ref("");
  const selectedLog = ref("");
  const logContent = ref("");
  const dataDirectory = ref("");
  const pendingDataDirectory = ref("");
  const autostartEnabled = ref(false);
  const autostartBusy = ref(false);

  const settingsPageHeading = computed(
    () =>
      ({
        general: { title: "常规", description: "开机自启动、铃声与窗口行为等基础设置" },
        storage: { title: "数据与备份", description: "选择数据存储位置，管理自动备份" },
        logs: { title: "日志", description: "错误与崩溃日志按日期保存，保留最近 7 天" },
        reset: { title: "重置数据", description: "选择需要清空的数据表，操作后不可恢复" },
      })[settingsTab.value],
  );

  const backupEnabled = computed(
    () => settings.value.autoBackupEnabled === "1",
  );

  // 去掉末尾斜杠后按平台拼接分隔符，兼容 Windows(\) 与 Linux(/)。
  const backupDirectory = computed(
    () =>
      dataDirectory.value.replace(/[\\/]$/, "") +
      (dataDirectory.value.includes("\\") ? "\\" : "/") +
      "backups",
  );

  // 启动时拉取配置并回填默认值。
  async function loadSettings() {
    settings.value = await api.settings();
    if (!settings.value.activityInterval)
      settings.value.activityInterval = "30";
    if (!settings.value.ringtoneVolume)
      settings.value.ringtoneVolume = "100";
    if (settings.value.startupWindow !== "tray")
      settings.value.startupWindow = "show";
    if (settings.value.closeBehavior !== "quit")
      settings.value.closeBehavior = "tray";
    ringtoneVolume.value = Number(settings.value.ringtoneVolume || "100");
  }

  // 拉取数据目录（启动加载时由编排方调用）。
  async function loadDataDirectory() {
    dataDirectory.value = await api.getDataDirectory();
  }

  // 打开常规 tab 并加载系统自启动状态。
  async function showGeneral() {
    settingsTab.value = "general";
    try {
      autostartEnabled.value = await api.getAutostartEnabled();
    } catch (e) {
      appStore.setError(e);
    }
  }

  // 保存单项常规设置：乐观更新，失败时回滚。
  async function saveGeneralSetting(key: string, value: string) {
    const previous = settings.value[key];
    if (previous === value) return;
    settings.value[key] = value;
    try {
      await api.saveSettings({ [key]: value });
    } catch (e) {
      settings.value[key] = previous;
      appStore.setError(e);
    }
  }

  function getNavigationWidth(level: NavigationLevel) {
    const key = navigationWidths[level].settingKey;
    return parseNavigationWidth(level, settings.value[key]);
  }

  let navigationWidthSave: Promise<void> = Promise.resolve();

  function saveNavigationWidth(level: NavigationLevel, width: number) {
    const key = navigationWidths[level].settingKey;
    const value = String(width);
    settings.value[key] = value;
    navigationWidthSave = navigationWidthSave
      .then(async () => {
        await api.saveSettings({ [key]: value });
      })
      .catch(appStore.setError);
    return navigationWidthSave;
  }

  // 铃声音量（0–100，0 为静音）：拖动即时反馈，停止调整 400ms 后保存。
  const ringtoneVolume = ref(100);
  let volumeSaveTimer: ReturnType<typeof setTimeout> | undefined;

  function setRingtoneVolume(volume: number) {
    ringtoneVolume.value = volume;
    clearTimeout(volumeSaveTimer);
    volumeSaveTimer = setTimeout(() => {
      void saveGeneralSetting(
        "ringtoneVolume",
        String(Math.max(0, Math.min(100, Math.round(volume)))),
      );
    }, 400);
  }

  // 启动后窗口行为与关闭按钮行为：切换即保存。
  const startupWindow = computed({
    get: () => (settings.value.startupWindow === "tray" ? "tray" : "show"),
    set: (value: string) => {
      void saveGeneralSetting("startupWindow", value);
    },
  });

  const closeBehavior = computed({
    get: () => (settings.value.closeBehavior === "quit" ? "quit" : "tray"),
    set: (value: string) => {
      void saveGeneralSetting("closeBehavior", value);
    },
  });

  async function toggleAutostart() {
    if (autostartBusy.value) return;
    autostartBusy.value = true;
    const next = !autostartEnabled.value;
    try {
      await api.setAutostartEnabled(next);
      autostartEnabled.value = next;
    } catch (e) {
      appStore.setError(e);
    } finally {
      autostartBusy.value = false;
    }
  }

  async function chooseDataDirectory() {
    try {
      const path = await api.chooseDataDirectory();
      if (path && path !== dataDirectory.value)
        pendingDataDirectory.value = path;
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function showResetTables() {
    settingsTab.value = "reset";
    try {
      resetTableList.value = await resetTables();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function confirmResetTable() {
    if (!resetTarget.value) return;
    try {
      await resetTable(resetTarget.value.name);
      resetTarget.value = null;
      await appStore.reload();
      resetTableList.value = await resetTables();
      appStore.notifyTop("数据已永久删除");
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function showLogs(fromRefresh = false) {
    settingsTab.value = "logs";
    try {
      const result = await errorLogs();
      logFiles.value = result.files;
      logDirectory.value = result.directory;
      if (
        selectedLog.value &&
        !result.files.some((f) => f.name === selectedLog.value)
      ) {
        selectedLog.value = "";
        logContent.value = "";
      }
      if (fromRefresh) appStore.notifyTop("日志已刷新");
    } catch (e) {
      if (fromRefresh) {
        appStore.notifyTop("日志刷新失败");
        recordClientError(e instanceof Error ? e.message : String(e)).catch(
          () => {},
        );
      } else appStore.setError(e);
    }
  }

  async function openLog(name: string) {
    try {
      selectedLog.value = name;
      logContent.value = (await readErrorLog(name)).content;
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function saveDataDirectory() {
    if (!pendingDataDirectory.value) return;
    try {
      await api.setDataDirectory(pendingDataDirectory.value);
      dataDirectory.value = await api.getDataDirectory();
      pendingDataDirectory.value = "";
      appStore.notifyTop("数据目录已更新，原有学习数据已迁移。");
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function toggleAutoBackup() {
    const previous = settings.value.autoBackupEnabled || "0";
    settings.value.autoBackupEnabled = backupEnabled.value ? "0" : "1";
    try {
      await api.saveSettings({ autoBackupEnabled: settings.value.autoBackupEnabled });
    } catch (e) {
      settings.value.autoBackupEnabled = previous;
      appStore.setError(e);
    }
  }

  return {
    settings,
    settingsTab,
    resetTableList,
    resetTarget,
    logFiles,
    logDirectory,
    selectedLog,
    logContent,
    dataDirectory,
    pendingDataDirectory,
    autostartEnabled,
    autostartBusy,
    settingsPageHeading,
    backupEnabled,
    backupDirectory,
    loadSettings,
    loadDataDirectory,
    showGeneral,
    saveGeneralSetting,
    getNavigationWidth,
    saveNavigationWidth,
    ringtoneVolume,
    setRingtoneVolume,
    startupWindow,
    closeBehavior,
    toggleAutostart,
    chooseDataDirectory,
    showResetTables,
    confirmResetTable,
    showLogs,
    openLog,
    saveDataDirectory,
    toggleAutoBackup,
  };
});
