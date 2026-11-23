import type { Ref } from "vue";
import type {
  DailyReview,
  ResetTable,
  Subject,
  Session,
  Plan,
} from "../../api";

// 键盘快捷键：处理各对话框的 Enter 提交、Escape 取消、Ctrl/Cmd+Enter 快速提交。
// 所有状态与提交方法通过参数注入，便于从顶层 composable 中独立出去。
type KeyboardDeps = {
  reviewPending: Ref<DailyReview | null>;
  reviewBulkChoice: Ref<"ask" | "unified" | "individual">;
  submitBulkReview: () => Promise<unknown> | void;
  submitReview: () => Promise<unknown> | void;
  openPlanNoteId: Ref<number>;
  reminderActive: Ref<boolean>;
  addActivity: () => Promise<unknown> | void;
  deletePlanTarget: Ref<unknown>;
  resetTarget: Ref<ResetTable | null>;
  deleteSubjectTarget: Ref<Subject | null>;
  deleteSessionTarget: Ref<Session | null>;
  showFinishConfirm: Ref<boolean>;
  pendingStartPlan: Ref<Plan | null>;
  showTaskReminder: Ref<boolean>;
  showBreakReminder: Ref<boolean>;
  showBreakExhausted: Ref<boolean>;
  showActivity: Ref<boolean>;
  showSubject: Ref<boolean>;
  addSubject: () => Promise<unknown> | void;
  showPlan: Ref<boolean>;
  addPlan: () => Promise<unknown> | void;
  showManual: Ref<boolean>;
  addManual: () => Promise<unknown> | void;
  setError: (error: unknown) => void;
};

export function useKeyboardShortcuts({
  reviewPending,
  reviewBulkChoice,
  submitBulkReview,
  submitReview,
  openPlanNoteId,
  reminderActive,
  addActivity,
  deletePlanTarget,
  resetTarget,
  deleteSubjectTarget,
  deleteSessionTarget,
  showFinishConfirm,
  pendingStartPlan,
  showTaskReminder,
  showBreakReminder,
  showBreakExhausted,
  showActivity,
  showSubject,
  addSubject,
  showPlan,
  addPlan,
  showManual,
  addManual,
  setError,
}: KeyboardDeps) {
  let keyboardSubmitting = false;

  function submitWithEnter(
    event: KeyboardEvent,
    action: () => Promise<unknown> | void,
  ) {
    event.preventDefault();
    if (keyboardSubmitting) return;
    keyboardSubmitting = true;
    try {
      Promise.resolve(action())
        .catch(setError)
        .finally(() => {
          keyboardSubmitting = false;
        });
    } catch (e) {
      keyboardSubmitting = false;
      setError(e);
    }
  }

  function handleDialogKeydown(event: KeyboardEvent) {
    if (reviewPending.value) {
      if (event.key === "Escape") {
        event.preventDefault();
        return;
      }
      if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
        if (reviewBulkChoice.value === "unified")
          submitWithEnter(event, submitBulkReview);
        else if (reviewBulkChoice.value === "individual")
          submitWithEnter(event, submitReview);
        return;
      }
      return;
    }
    if (event.key === "Escape") {
      openPlanNoteId.value = 0;
      return;
    }
    if (
      event.key !== "Enter" ||
      event.isComposing ||
      event.keyCode === 229 ||
      event.repeat ||
      event.altKey ||
      event.shiftKey
    )
      return;
    const target = event.target;
    if (target instanceof HTMLElement && target.isContentEditable) return;
    if (
      target instanceof HTMLSelectElement ||
      (target instanceof HTMLInputElement &&
        ["color", "date", "time"].includes(target.type))
    )
      return;
    if (reminderActive.value) {
      if (
        target instanceof HTMLButtonElement &&
        target.closest(".activity-reminder-backdrop")
      )
        return;
      submitWithEnter(event, addActivity);
      return;
    }
    if (
      target instanceof HTMLButtonElement &&
      target.closest(".plan-dialog-card, .subject-form-dialog")
    ) return;
    if (
      resetTarget.value ||
      deletePlanTarget.value ||
      deleteSubjectTarget.value ||
      deleteSessionTarget.value ||
      showFinishConfirm.value
    ) {
      event.preventDefault();
      return;
    }
    if (
      pendingStartPlan.value ||
      showTaskReminder.value ||
      showBreakReminder.value ||
      showBreakExhausted.value
    )
      return;
    if (target instanceof HTMLButtonElement && target.closest(".modal")) return;
    if (
      target instanceof HTMLTextAreaElement &&
      showPlan.value &&
      !event.ctrlKey &&
      !event.metaKey
    )
      return;
    if (
      !(target instanceof HTMLTextAreaElement) &&
      (event.ctrlKey || event.metaKey)
    )
      return;
    if (showActivity.value) {
      submitWithEnter(event, addActivity);
      return;
    }
    if (showSubject.value) {
      submitWithEnter(event, addSubject);
      return;
    }
    if (showPlan.value) {
      submitWithEnter(event, addPlan);
      return;
    }
    if (showManual.value) submitWithEnter(event, addManual);
  }

  return { handleDialogKeydown };
}
