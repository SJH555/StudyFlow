import { ref } from "vue";
import type { Plan } from "../../api";
import { colorPalette } from "../../shared/constants";
import { useSubjectDraft } from "../subjects/useSubjectDraft";

// 计划草稿：表单字段、确认状态与备注展开状态；页面与弹窗共享，
// 应用生命周期内保留（切页不清空），由业务模块封装，不进 Pinia。
const planSubject = ref(0);
const planColor = ref(colorPalette[0]);
const planTime = ref("09:00");
const planHour = ref("09");
const planMinute = ref("00");
const planDuration = ref("60");
const planBreakEnabled = ref(false);
const planBreakMinutes = ref("20");
const planNote = ref("");
const planEditId = ref(0);
const repeatMode = ref("daily");
const selectedWeekdays = ref<number[]>([1, 2, 3, 4, 5]);
const showPlanColors = ref(false);
const showPlan = ref(false);
const deletePlanTarget = ref<Plan | null>(null);
const deletePlanDay = ref("");
const savingPlan = ref(false);
const planProjectFilter = ref(0);
const openPlanNoteId = ref(0);
const openScheduleNoteId = ref(0);

// 切换计划备注弹窗。
function togglePlanNote(id: number) {
  openPlanNoteId.value = openPlanNoteId.value === id ? 0 : id;
}

// 点击备注弹窗外部时关闭全部备注弹窗（含科目视图的备注）。
function closePlanNoteOutside(event: PointerEvent) {
  if (
    !(event.target instanceof Element) ||
    event.target.closest(
      ".plan-note-chip,.today-note-tooltip",
    )
  )
    return;
  openPlanNoteId.value = 0;
  openScheduleNoteId.value = 0;
  useSubjectDraft().openSubjectPlanNoteId.value = 0;
}

export function usePlanDraft() {
  return {
    planSubject,
    planColor,
    planTime,
    planHour,
    planMinute,
    planDuration,
    planBreakEnabled,
    planBreakMinutes,
    planNote,
    planEditId,
    repeatMode,
    selectedWeekdays,
    showPlanColors,
    showPlan,
    deletePlanTarget,
    deletePlanDay,
    savingPlan,
    planProjectFilter,
    openPlanNoteId,
    openScheduleNoteId,
    togglePlanNote,
    closePlanNoteOutside,
  };
}
