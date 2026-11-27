import { computed, ref, watch } from "vue";
import type { ManualPlanConflict, Subject } from "../../api";

// 手动记录草稿：表单字段，弹窗与页面共享，应用生命周期内保留，由业务模块封装。
const manualDate = ref("");
const manualSubjects = ref<Subject[]>([]);
const manualSubjectsLoading = ref(false);
const manualSaving = ref(false);
const manualSubject = ref(0);
const manualStartHour = ref("09");
const manualStartMinute = ref("00");
const manualEndHour = ref("10");
const manualEndMinute = ref("00");
const showManual = ref(false);
const manualPlanConflict = ref<ManualPlanConflict | null>(null);

watch([
  showManual, manualDate, manualSubject, manualStartHour, manualStartMinute,
  manualEndHour, manualEndMinute,
], () => { manualPlanConflict.value = null; }, { flush: "sync" });
const manualDurationMinutes = computed(() =>
  Number(manualEndHour.value) * 60 + Number(manualEndMinute.value) -
  Number(manualStartHour.value) * 60 - Number(manualStartMinute.value),
);
const manualTimeError = computed(() => {
  const duration = manualDurationMinutes.value;
  if (!Number.isFinite(duration) || duration <= 0) return "结束时间必须晚于开始时间";
  return duration < 10 ? "学习记录不能少于 10 分钟，请调整时间" : "";
});

export function useManualDraft() {
  return {
    manualDate,
    manualSubjects,
    manualSubjectsLoading,
    manualSaving,
    manualPlanConflict,
    manualSubject,
    manualStartHour,
    manualStartMinute,
    manualEndHour,
    manualEndMinute,
    showManual,
    manualDurationMinutes,
    manualTimeError,
  };
}
