import { ref } from "vue";

// 手动记录草稿：表单字段，弹窗与页面共享，应用生命周期内保留，由业务模块封装。
const manualSubject = ref(0);
const manualStartHour = ref("09");
const manualStartMinute = ref("00");
const manualEndHour = ref("10");
const manualEndMinute = ref("00");
const showManual = ref(false);

export function useManualDraft() {
  return {
    manualSubject,
    manualStartHour,
    manualStartMinute,
    manualEndHour,
    manualEndMinute,
    showManual,
  };
}
