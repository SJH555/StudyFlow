import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { api, type Plan, type Session } from "../api";
import { useAppStore } from "./app";
import { useSettingsStore } from "./settings";
import { usePlansStore } from "./plans";
import { dateKey, formatDateChinese, parseDateKey } from "../shared/format";
import { useManualDraft } from "../features/sessions/useManualDraft";

// 学习记录：记录数据与进行中会话的所有者，负责开始/暂停/恢复/结束、
// 跨计划提醒、任务/休息提醒状态与手动补录。
export const useSessionsStore = defineStore("sessions", () => {
  const appStore = useAppStore();
  const settingsStore = useSettingsStore();
  const plansStore = usePlansStore();
  const manual = useManualDraft();

  const sessions = ref<Session[]>([]);
  const liveSession = ref<Session | null>(null);
  let selectedLoadSequence = 0;
  let todayLoadSequence = 0;
  let manualDateSequence = 0;
  let stopping = false;
  const startingPlanId = ref(0);
  const pendingStartPlan = ref<Plan | null>(null);
  const showFinishConfirm = ref(false);
  const deleteSessionTarget = ref<Session | null>(null);
  const showBreakExhausted = ref(false);
  const showBreakReminder = ref(false);
  const breakReminderSubject = ref("");
  const showTaskReminder = ref(false);
  const taskReminderEnded = ref<{ name: string; saved: boolean } | null>(null);

  const active = computed(() => liveSession.value);

  // 今日是否暂停计划（设置里记录暂停日期与今天一致）。
  const todayPaused = computed(
    () => settingsStore.settings.pausedPlanDate === appStore.todayKey,
  );

  function publishSelected(date: string, sequence: number, rows: Session[]) {
    if (sequence === selectedLoadSequence && date === appStore.selectedDate) {
      sessions.value = rows;
    }
  }

  // 今日的进行中会话与选中日期列表分别更新，历史日期请求不影响计时器。
  async function refreshToday(updateSelected = appStore.page === "today") {
    const today = appStore.todayKey;
    const todaySequence = ++todayLoadSequence;
    const selectedSequence = updateSelected && today === appStore.selectedDate
      ? ++selectedLoadSequence
      : null;
    const todaySessions = await api.sessions(today);
    appStore.tick();
    if (todaySequence === todayLoadSequence && today === appStore.todayKey) {
      liveSession.value = todaySessions.find((s) => !s.endAt) || null;
    }
    if (selectedSequence !== null && today === appStore.todayKey) {
      publishSelected(today, selectedSequence, todaySessions);
    }
    return todaySessions;
  }

  // 按日期拉取记录（页面切换/日期切换时由编排方调用）。
  async function fetchForDate(d: string) {
    const sequence = ++selectedLoadSequence;
    const rows = await api.sessions(d);
    publishSelected(d, sequence, rows);
    return rows;
  }

  async function loadInitial() {
    const selected = appStore.selectedDate;
    if (selected === appStore.todayKey) {
      await refreshToday(true);
      return;
    }
    const sequence = selectedLoadSequence;
    await refreshToday(false);
    if (selected === appStore.selectedDate && sequence === selectedLoadSequence) {
      await fetchForDate(selected);
    }
  }

  function openManual() {
    if (manual.manualSaving.value) return;
    const now = new Date();
    const end = Math.max(10, now.getHours() * 60 + now.getMinutes());
    const start = Math.max(0, end - 60);
    manual.manualStartHour.value = String(Math.floor(start / 60)).padStart(2, "0");
    manual.manualStartMinute.value = String(start % 60).padStart(2, "0");
    manual.manualEndHour.value = String(Math.floor(end / 60)).padStart(2, "0");
    manual.manualEndMinute.value = String(end % 60).padStart(2, "0");
    manual.manualSubject.value = 0;
    void chooseManualDate(dateKey(now));
    manual.showManual.value = true;
  }

  async function chooseManualDate(value: unknown) {
    const date = parseDateKey(value);
    if (!date || manual.manualSaving.value) return;
    const sequence = ++manualDateSequence;
    const previousSubject = manual.manualSubject.value;
    manual.manualDate.value = date;
    manual.manualSubjects.value = [];
    manual.manualSubjectsLoading.value = true;
    try {
      const items = await api.subjectsForManualDate(date);
      if (sequence !== manualDateSequence) return;
      manual.manualSubjects.value = items;
      manual.manualSubject.value = items.some((item) => item.id === previousSubject)
        ? previousSubject
        : items[0]?.id ?? 0;
    } catch (e) {
      if (sequence === manualDateSequence) appStore.setError(e);
    } finally {
      if (sequence === manualDateSequence) manual.manualSubjectsLoading.value = false;
    }
  }

  function startCurrent() {
    if (!plansStore.currentPlan) {
      appStore.notifyTop("当前没有正在进行的学习计划");
      return;
    }
    start(plansStore.currentPlan);
  }

  async function start(p: Plan) {
    if (startingPlanId.value || active.value?.planId === p.id) return;
    if (todayPaused.value) {
      appStore.notifyTop("今日计划已暂停，请先恢复");
      return;
    }
    if (plansStore.hasPlanEnded(p)) {
      appStore.notifyTop("该学习时段已结束，无法开始");
      return;
    }
    if (active.value) {
      appStore.notifyTop("请先结束当前学习");
      return;
    }
    startingPlanId.value = p.id;
    try {
      const todaySessions = await refreshToday();
      if (todaySessions.find((s) => !s.endAt)?.planId === p.id) return;
      // 若更早的时段尚未开始且没有有效记录，先提示用户，避免跳过前序计划。
      const skipped = plansStore.todayPlans.filter(
        (earlier) =>
          earlier.startTime < p.startTime &&
          !plansStore.hasPlanEnded(earlier) &&
          !todaySessions.some(
            (s) => s.planId === earlier.id && s.endAt && s.duration >= 600,
          ),
      );
      if (skipped.length) {
        pendingStartPlan.value = p;
        return;
      }
      await startPlanNow(p);
    } catch (e) {
      appStore.setError(e);
    } finally {
      startingPlanId.value = 0;
    }
  }

  async function startPlanNow(p: Plan) {
    if (active.value?.planId === p.id) return;
    startingPlanId.value = p.id;
    try {
      if (todayPaused.value) {
        appStore.notifyTop("今日计划已暂停，请先恢复");
        return;
      }
      if (!plansStore.todayPlans.some((plan) => plan.id === p.id)) {
        pendingStartPlan.value = null;
        appStore.notifyTop("这个计划已不在今日安排中");
        return;
      }
      if (plansStore.hasPlanEnded(p)) {
        pendingStartPlan.value = null;
        appStore.notifyTop("该学习时段已结束，无法开始");
        return;
      }
      if (active.value) {
        appStore.notifyTop("请先结束当前学习");
        return;
      }
      await api.start(p.subjectId, p.id);
      await refreshToday();
      pendingStartPlan.value = null;
    } catch (e) {
      appStore.setError(e);
    } finally {
      startingPlanId.value = 0;
    }
  }

  async function toggleTodayPause() {
    const previous = settingsStore.settings.pausedPlanDate || "";
    try {
      if (!todayPaused.value && active.value)
        await api.finish(active.value.id, "pause-day");
      settingsStore.settings.pausedPlanDate = todayPaused.value
        ? ""
        : appStore.todayKey;
      await api.saveSettings({ pausedPlanDate: settingsStore.settings.pausedPlanDate });
      await refreshToday();
      appStore.notifyTop(todayPaused.value ? "今日计划已暂停" : "今日计划已恢复");
    } catch (e) {
      settingsStore.settings.pausedPlanDate = previous;
      appStore.setError(e);
    }
  }

  async function stop() {
    const session = active.value;
    if (!session || stopping) return false;
    stopping = true;
    try {
      const result = await api.finish(session.id);
      appStore.tick();
      showFinishConfirm.value = false;
      if (liveSession.value?.id === session.id) liveSession.value = null;
      if (appStore.focusMode) appStore.exitFocus();
      if (!result.saved) appStore.notifyTop("本次不足 10 分钟，未计入学习记录");

      await refreshToday().catch((error) => {
        // 已确认结束的记录不能在刷新失败时继续显示为进行中。
        sessions.value = sessions.value.filter((row) => row.id !== session.id || row.endAt);
        appStore.setError(error);
      });
      return true;
    } catch (e) {
      appStore.setError(e);
      return false;
    } finally {
      stopping = false;
    }
  }

  async function pause() {
    if (!active.value || active.value.pausedAt) return;
    try {
      await api.finish(active.value.id, "pause");
      await refreshToday();
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      // 后端返回“休息时间已用完”时单独展示提示，而不是普通错误。
      if (message.includes("休息时间已用完")) showBreakExhausted.value = true;
      else appStore.notifyTop(message);
    }
  }

  async function resume() {
    if (!active.value || !active.value.pausedAt) return;
    if (todayPaused.value) {
      appStore.notifyTop("今日计划已暂停，请先恢复");
      return;
    }
    try {
      await api.finish(active.value.id, "resume");
      await refreshToday();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function finishBreakReminder(resumeNow: boolean) {
    try {
      if (resumeNow) {
        if (todayPaused.value) {
          appStore.notifyTop("今日计划已暂停，请先恢复");
          return;
        }
        const current = (await refreshToday()).find(
          (s) => !s.endAt && s.pausedAt,
        );
        if (current) await api.finish(current.id, "resume");
        await refreshToday();
      }
      showBreakReminder.value = false;
      await appStore.stopReminderSound();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function dismissTaskReminder() {
    showTaskReminder.value = false;
    taskReminderEnded.value = null;
    await appStore.stopReminderSound();
  }

  async function addManual() {
    if (manual.manualSaving.value || manual.manualSubjectsLoading.value) return;
    const day = manual.manualDate.value;
    if (!day) return;
    manual.manualSaving.value = true;
    try {
      const startTime =
          manual.manualStartHour.value + ":" + manual.manualStartMinute.value,
        endTime = manual.manualEndHour.value + ":" + manual.manualEndMinute.value;
      if (!manual.manualSubjects.value.some((item) => item.id === manual.manualSubject.value)) {
        appStore.notifyTop("请选择学习项目");
        return;
      }
      if (manual.manualTimeError.value) {
        appStore.notifyTop(manual.manualTimeError.value);
        return;
      }
      const start = new Date(`${day}T${startTime}:00+08:00`),
        end = new Date(`${day}T${endTime}:00+08:00`);
      if (end <= start) {
        appStore.notifyTop("结束时间必须晚于开始时间");
        return;
      }
      await api.addSession({
        subjectId: manual.manualSubject.value,
        startAt: start.toISOString(),
        endAt: end.toISOString(),
      });
      manual.showManual.value = false;
      appStore.notifyTop(`已补录 ${formatDateChinese(day)} 的学习记录`);
      if (day === appStore.selectedDate) await fetchForDate(day).catch(appStore.setError);
    } catch (e) {
      appStore.notifyTop(e instanceof Error ? e.message : String(e));
    } finally {
      manual.manualSaving.value = false;
    }
  }

  // 进行中的学习（未结束且未暂停）需实时累计：已计秒数 + 本次恢复以来的流逝秒数。
  function sessionSeconds(s: Session) {
    if (s.endAt || s.pausedAt) return s.duration;
    return (
      s.duration +
      Math.max(
        0,
        Math.floor(
          (appStore.now - new Date(s.resumedAt || s.startAt).getTime()) / 1000,
        ),
      )
    );
  }

  async function deleteSession() {
    if (!deleteSessionTarget.value) return;
    try {
      await api.deleteSession(deleteSessionTarget.value.id);
      deleteSessionTarget.value = null;
      await refreshToday();
    } catch (e) {
      appStore.setError(e);
    }
  }

  return {
    sessions,
    liveSession,
    active,
    todayPaused,
    startingPlanId,
    pendingStartPlan,
    showFinishConfirm,
    deleteSessionTarget,
    showBreakExhausted,
    showBreakReminder,
    breakReminderSubject,
    showTaskReminder,
    taskReminderEnded,
    refreshToday,
    loadInitial,
    fetchForDate,
    openManual,
    chooseManualDate,
    startCurrent,
    start,
    startPlanNow,
    toggleTodayPause,
    stop,
    pause,
    resume,
    finishBreakReminder,
    dismissTaskReminder,
    addManual,
    deleteSession,
    sessionSeconds,
  };
});
