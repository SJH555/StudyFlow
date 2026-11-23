import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  api,
  recordClientError,
  pendingDailyReview,
  dailyReviewHistory,
  submitDailyReview,
  submitBulkDailyReview,
  type DailyReview,
  type BulkReviewOffer,
} from "../api";
import { useAppStore } from "./app";
import { useSettingsStore } from "./settings";

// 每日复习（进度追因）：待处理项、草稿原因、历史记录与设置的所有者，
// 负责每日完成情况检查、未达标原因填写与设置保存。
export const useReviewStore = defineStore("review", () => {
  const appStore = useAppStore();
  const settingsStore = useSettingsStore();

  const summaryTab = ref<"goals" | "learning">("goals");
  const reviewPending = ref<DailyReview | null>(null);
  const reviewDraft = ref<Record<number, string>>({});
  const reviewHistory = ref<DailyReview[]>([]);
  const reviewMode = ref<"total" | "project">("total");
  const reviewThreshold = ref<number | null>(50); // 默认目标完成比例 50%（百分比）
  const reviewCheckHour = ref<number | null>(6); // 检查前一天完成情况的小时（0–23）
  const reviewCheckMinute = ref<number | null>(0); // 检查前一天完成情况的分钟（0–59）
  const reviewEnabledDraft = ref(false);
  const reviewSaving = ref(false);

  // 复习保存结果提示：开关、文案与成功/失败色调。
  const reviewSaveNotice = ref(false);
  const reviewSaveMessage = ref("");
  const reviewSaveTone = ref<"success" | "error">("success");

  // 复习设置的保存中标记（活动设置的已迁入 activity store）。
  const reviewSettingsSaving = ref(false);

  // 批量复习：可选方案、用户选择与统一补填原因。
  const reviewBulkOffer = ref<BulkReviewOffer | null>(null);
  const reviewBulkChoice = ref<"ask" | "unified" | "individual">("individual");
  const reviewBulkReason = ref("");

  const reviewPageHeading = computed(() => ({
    title: "学情追踪",
    description: "设定阶段目标，回顾学习进展并持续调整。",
  }));

  const reviewSettingsDirty = computed(
    () =>
      reviewEnabledDraft.value !==
        (settingsStore.settings.dailyReviewEnabled === "1") ||
      reviewMode.value !==
        (settingsStore.settings.dailyReviewMode === "project"
          ? "project"
          : "total") ||
      reviewThreshold.value !==
        Number(settingsStore.settings.dailyReviewThreshold || "50") ||
      reviewCheckHour.value !==
        Number(settingsStore.settings.dailyReviewCheckHour || "6") ||
      reviewCheckMinute.value !==
        Number(settingsStore.settings.dailyReviewCheckMinute || "0"),
  );

  // 启动加载设置后回填复习表单草稿。
  function applySettings() {
    reviewMode.value =
      settingsStore.settings.dailyReviewMode === "project" ? "project" : "total";
    reviewThreshold.value = Number(
      settingsStore.settings.dailyReviewThreshold || "50",
    );
    reviewCheckHour.value = Number(
      settingsStore.settings.dailyReviewCheckHour || "6",
    );
    reviewCheckMinute.value = Number(
      settingsStore.settings.dailyReviewCheckMinute || "0",
    );
    reviewEnabledDraft.value = settingsStore.settings.dailyReviewEnabled === "1";
  }

  let reviewCheckSequence = 0;

  function showReviewSaveNotice(message: string, tone: "success" | "error") {
    reviewSaveMessage.value = message;
    reviewSaveTone.value = tone;
    reviewSaveNotice.value = true;
  }

  // 用序号丢弃过期的并发请求结果，避免旧数据覆盖新状态。
  async function checkDailyReview() {
    const sequence = ++reviewCheckSequence;
    try {
      const result = await pendingDailyReview();
      if (sequence !== reviewCheckSequence) return;
      // 待填日期变化时清空草稿，避免串到其他日期。
      if (result.pending?.day !== reviewPending.value?.day) {
        reviewDraft.value = {};
        for (const item of result.pending?.items || [])
          reviewDraft.value[item.subjectId] = "";
      }
      if (
        result.bulk?.startDay !== reviewBulkOffer.value?.startDay ||
        result.bulk?.endDay !== reviewBulkOffer.value?.endDay
      ) {
        reviewBulkChoice.value = result.bulk ? "ask" : "individual";
        reviewBulkReason.value = "";
      }
      reviewBulkOffer.value = result.bulk;
      reviewPending.value = result.pending;
      if (result.pending && appStore.focusMode) appStore.exitFocus();
      if (appStore.page === "summary") await refreshReviewHistory();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function refreshReviewHistory() {
    try {
      reviewHistory.value = await dailyReviewHistory();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function saveReviewSettings() {
    if (!reviewSettingsDirty.value || reviewSettingsSaving.value) return;
    const threshold = reviewThreshold.value;
    if (
      !Number.isInteger(threshold) ||
      threshold === null ||
      threshold < 1 ||
      threshold > 100
    ) {
      showReviewSaveNotice("目标比例请输入 1–100 的整数", "error");
      return;
    }
    const checkHour = reviewCheckHour.value;
    if (
      !Number.isInteger(checkHour) ||
      checkHour === null ||
      checkHour < 0 ||
      checkHour > 23
    ) {
      showReviewSaveNotice("检查时间请输入 0–23 的小时", "error");
      return;
    }
    const checkMinute = reviewCheckMinute.value;
    if (
      !Number.isInteger(checkMinute) ||
      checkMinute === null ||
      checkMinute < 0 ||
      checkMinute > 59
    ) {
      showReviewSaveNotice("检查分钟请输入 0–59 的整数", "error");
      return;
    }
    reviewSettingsSaving.value = true;
    const saved = {
      dailyReviewEnabled: reviewEnabledDraft.value ? "1" : "0",
      dailyReviewMode: reviewMode.value,
      dailyReviewThreshold: String(threshold),
      dailyReviewCheckHour: String(checkHour),
      dailyReviewCheckMinute: String(checkMinute),
    };
    try {
      await api.saveSettings(saved);
      Object.assign(settingsStore.settings, saved);
      showReviewSaveNotice("进度追因设置保存成功", "success");
      await checkDailyReview();
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      recordClientError(message || "保存进度追因设置失败").catch(() => {});
      showReviewSaveNotice(
        message ? `保存失败：${message}` : "保存失败，请稍后重试",
        "error",
      );
    } finally {
      reviewSettingsSaving.value = false;
    }
  }

  async function submitReview() {
    const current = reviewPending.value;
    if (!current || reviewSaving.value) return;
    const reasons = current.items.map((item) => ({
      ...item,
      reason: (reviewDraft.value[item.subjectId] || "").trim(),
    }));
    if (reasons.some((item) => !item.reason)) {
      appStore.notifyTop("请填写每项未达标的理由");
      return;
    }
    reviewSaving.value = true;
    try {
      await submitDailyReview(current.day, reasons);
      reviewPending.value = null;
      await checkDailyReview();
    } catch (e) {
      appStore.setError(e);
    } finally {
      reviewSaving.value = false;
    }
  }

  async function submitBulkReview() {
    const offer = reviewBulkOffer.value;
    if (!offer || reviewSaving.value) return;
    const reason = reviewBulkReason.value.trim();
    if (!reason) {
      appStore.notifyTop("请填写统一原因");
      return;
    }
    reviewSaving.value = true;
    try {
      await submitBulkDailyReview(offer.startDay, offer.endDay, reason);
      reviewPending.value = null;
      reviewBulkOffer.value = null;
      reviewBulkChoice.value = "individual";
      reviewBulkReason.value = "";
      await checkDailyReview();
    } catch (e) {
      appStore.setError(e);
    } finally {
      reviewSaving.value = false;
    }
  }

  return {
    summaryTab,
    reviewPending,
    reviewDraft,
    reviewHistory,
    reviewMode,
    reviewThreshold,
    reviewCheckHour,
    reviewCheckMinute,
    reviewEnabledDraft,
    reviewSaving,
    reviewSaveNotice,
    reviewSaveMessage,
    reviewSaveTone,
    reviewSettingsSaving,
    reviewBulkOffer,
    reviewBulkChoice,
    reviewBulkReason,
    reviewPageHeading,
    reviewSettingsDirty,
    applySettings,
    showReviewSaveNotice,
    checkDailyReview,
    refreshReviewHistory,
    saveReviewSettings,
    submitReview,
    submitBulkReview,
  };
});
