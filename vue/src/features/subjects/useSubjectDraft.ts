import { ref } from "vue";
import type { Subject } from "../../api";

// 科目草稿与科目页视图状态：页面与弹窗共享，应用生命周期内保留（切页不清空），
// 由业务模块封装，不进 Pinia。
const subjectEditId = ref(0);
const subjectName = ref("");
const subjectColor = ref("");
const subjectSortEnabled = ref(false);
const subjectSortInput = ref("");
const showSubjectColors = ref(false);
const showSubject = ref(false);
const deleteSubjectTarget = ref<Subject | null>(null);
const subjectTab = ref<"enabled" | "hidden">("enabled");
const subjectPlanTarget = ref<Subject | null>(null);
const showSubjectPlans = ref(false);
const openSubjectPlanNoteId = ref(0);

export function useSubjectDraft() {
  return {
    subjectEditId,
    subjectName,
    subjectColor,
    subjectSortEnabled,
    subjectSortInput,
    showSubjectColors,
    showSubject,
    deleteSubjectTarget,
    subjectTab,
    subjectPlanTarget,
    showSubjectPlans,
    openSubjectPlanNoteId,
  };
}
