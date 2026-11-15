import { api, type Plan } from "../../api";
import { conflictingPlan } from "./conflicts";
import { usePlanDraft } from "./usePlanDraft";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlansStore } from "../../stores/plans";
import { useAppStore } from "../../stores/app";
import { colorPalette, weekNames } from "../../shared/constants";

// 学习计划表单：负责计划表单初始值、校验、冲突检测与增删改提交。
// 提交后的项目+计划联合刷新通过 subjects store 的公开能力完成。
export function usePlansForm() {
  const draft = usePlanDraft();
  const subjectsStore = useSubjectsStore();
  const plansStore = usePlansStore();
  const appStore = useAppStore();

  function beginAddPlan() {
    draft.planEditId.value = 0;
    draft.repeatMode.value = "daily";
    draft.selectedWeekdays.value = [1, 2, 3, 4, 5];
    draft.planSubject.value = subjectsStore.subjects[0]?.id || 0;
    draft.planHour.value = String(new Date().getHours()).padStart(2, "0");
    draft.planMinute.value = String(new Date().getMinutes()).padStart(2, "0");
    draft.planTime.value = draft.planHour.value + ":" + draft.planMinute.value;
    draft.planDuration.value = "60";
    draft.planBreakEnabled.value = false;
    draft.planBreakMinutes.value = "20";
    draft.planNote.value = "";
    draft.planColor.value =
      colorPalette[plansStore.plans.length % colorPalette.length];
    draft.showPlanColors.value = false;
    draft.showPlan.value = true;
  }

  function beginEditPlan(p: Plan) {
    draft.planEditId.value = p.id;
    draft.planSubject.value = p.subjectId;
    draft.planHour.value = p.startTime.slice(0, 2);
    draft.planMinute.value = p.startTime.slice(3, 5);
    draft.planDuration.value = String(p.duration);
    draft.planBreakEnabled.value = (p.breakMinutes || 0) > 0;
    draft.planBreakMinutes.value = String(p.breakMinutes || 20);
    draft.planNote.value = p.note || "";
    draft.planColor.value = p.color;
    draft.selectedWeekdays.value = p.weekdays.split(",").map(Number);
    // 根据所选星期反推重复模式：全 7 天=每天，仅工作日=工作日，其余=自定义。
    draft.repeatMode.value =
      draft.selectedWeekdays.value.length === 7
        ? "daily"
        : draft.selectedWeekdays.value.join(",") === "1,2,3,4,5"
          ? "weekdays"
          : "custom";
    draft.showPlanColors.value = false;
    draft.showPlan.value = true;
  }

  function toggleWeekday(day: number) {
    draft.selectedWeekdays.value = draft.selectedWeekdays.value.includes(day)
      ? draft.selectedWeekdays.value.filter((x) => x !== day)
      : [...draft.selectedWeekdays.value, day].sort((a, b) => a - b);
  }

  function addPlan() {
    draft.planTime.value = draft.planHour.value + ":" + draft.planMinute.value;
    const duration = Number(draft.planDuration.value);
    const breakMinutes = draft.planBreakEnabled.value
      ? Number(draft.planBreakMinutes.value)
      : 0;
    const weekdays =
      draft.repeatMode.value === "daily"
        ? "0,1,2,3,4,5,6"
        : draft.repeatMode.value === "weekdays"
          ? "1,2,3,4,5"
          : draft.selectedWeekdays.value.join(",");
    if (!weekdays) {
      appStore.setError("至少选择一个星期");
      return;
    }
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(draft.planTime.value)) {
      appStore.setError("请选择有效的开始时间");
      return;
    }
    if (Number.isFinite(duration) && duration <= 0) {
      appStore.setError("结束时间必须晚于开始时间");
      return;
    }
    // 1440 为一整天分钟数，限制计划时长不能超过一天。
    if (
      !/^\d+$/.test(draft.planDuration.value.trim()) ||
      !Number.isSafeInteger(duration) ||
      duration < 1 ||
      duration > 1440
    ) {
      appStore.setError("计划时长请输入 1 到 1440 之间的整数分钟");
      return;
    }
    const startMinutes =
      Number(draft.planHour.value) * 60 + Number(draft.planMinute.value);
    if (startMinutes + duration >= 1440) {
      appStore.setError("计划需在当天结束，请调整时间或缩短时长");
      return;
    }
    if (
      draft.planBreakEnabled.value &&
      (!/^\d+$/.test(draft.planBreakMinutes.value.trim()) ||
        !Number.isSafeInteger(breakMinutes) ||
        breakMinutes < 1 ||
        breakMinutes >= duration)
    ) {
      appStore.setError("休息时长需为正整数，且小于计划时长");
      return;
    }
    const note = draft.planNote.value.trim();
    // 按字符（含中文）计数，上限 100 字。
    if (Array.from(note).length > 100) {
      appStore.notifyTop("计划备注最多 100 字");
      return;
    }
    const p: Plan = {
      id: draft.planEditId.value,
      subjectId: draft.planSubject.value,
      startTime: draft.planTime.value,
      duration,
      breakMinutes,
      color: draft.planColor.value,
      weekdays,
      active: true,
      subject:
        subjectsStore.subjects.find(
          (s) => s.id === draft.planSubject.value,
        )?.name || "",
      note,
    };
    const conflict = conflictingPlan(p, plansStore.plans, weekNames);
    if (conflict) {
      appStore.notifyTop(conflict);
      return;
    }
    draft.pendingPlan.value = p;
    draft.pendingPlanAction.value = draft.planEditId.value ? "edit" : "create";
    draft.showPlan.value = false;
  }

  function cancelPlanAction() {
    if (
      draft.pendingPlanAction.value === "create" ||
      draft.pendingPlanAction.value === "edit"
    )
      draft.showPlan.value = true;
    draft.pendingPlanAction.value = null;
    draft.pendingPlan.value = null;
  }

  function deletePlan(id: number) {
    const p = plansStore.plans.find((x) => x.id === id);
    if (p) {
      draft.pendingPlan.value = p;
      draft.pendingPlanAction.value = "delete";
    }
  }

  async function confirmPlanAction() {
    const p = draft.pendingPlan.value,
      action = draft.pendingPlanAction.value;
    if (!p || !action) return;
    try {
      if (action === "create") await api.addPlan(p);
      else if (action === "edit") await api.updatePlan(p);
      else await api.deletePlan(p.id);
      await subjectsStore.refreshProjectAndPlan();
      draft.pendingPlanAction.value = null;
      draft.pendingPlan.value = null;
      draft.showPlan.value = false;
    } catch (e) {
      appStore.setError(e);
    }
  }

  return {
    beginAddPlan,
    beginEditPlan,
    toggleWeekday,
    addPlan,
    cancelPlanAction,
    deletePlan,
    confirmPlanAction,
  };
}
