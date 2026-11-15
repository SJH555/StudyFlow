import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { api, type Subject } from "../api";
import { colorPalette } from "../shared/constants";
import { useAppStore } from "./app";
import { usePlansStore } from "./plans";
import { useSubjectDraft } from "../features/subjects/useSubjectDraft";
import { usePlanDraft } from "../features/plans/usePlanDraft";

// 学习科目：科目数据的所有者，负责增删改、归档/恢复、排序号与科目计划联合刷新。
export const useSubjectsStore = defineStore("subjects", () => {
  const appStore = useAppStore();
  const plansStore = usePlansStore();
  const draft = useSubjectDraft();
  const planDraft = usePlanDraft();

  const subjects = ref<Subject[]>([]);
  const allSubjects = ref<Subject[]>([]);

  // 下一个可用科目排序号（新增科目默认排最后）。
  const nextSubjectSortOrder = computed(
    () => Math.max(0, ...allSubjects.value.map((s) => s.sortOrder || 0)) + 1,
  );

  async function fetchAll() {
    [subjects.value, allSubjects.value] = await Promise.all([
      api.subjects(),
      api.subjects(true, true),
    ]);
  }

  // 科目与计划的联合刷新：并行请求，全部成功后才一起更新（不出现部分列表提前更新），
  // 并校正失效的项目筛选。计划的写入通过 plans store 的公开能力完成。
  async function refreshProjectAndPlan() {
    const [enabled, all, currentPlans] = await Promise.all([
      api.subjects(),
      api.subjects(true, true),
      api.plans(),
    ]);
    subjects.value = enabled;
    allSubjects.value = all;
    plansStore.setPlans(currentPlans);
    if (
      planDraft.planProjectFilter.value &&
      !all.some(
        (subject) =>
          subject.id === planDraft.planProjectFilter.value && !subject.deleted,
      )
    )
      planDraft.planProjectFilter.value = 0;
  }

  function beginAddSubject() {
    draft.subjectEditId.value = 0;
    draft.subjectName.value = "";
    draft.subjectColor.value =
      colorPalette[allSubjects.value.length % colorPalette.length];
    draft.subjectSortEnabled.value = false;
    draft.subjectSortInput.value = "";
    draft.showSubjectColors.value = false;
    draft.showSubject.value = true;
  }

  function beginEditSubject(subject: Subject) {
    draft.subjectEditId.value = subject.id;
    draft.subjectName.value = subject.name;
    draft.subjectColor.value = subject.color;
    draft.subjectSortEnabled.value = subject.sortOrder !== null;
    draft.subjectSortInput.value =
      subject.sortOrder === null ? "" : String(subject.sortOrder);
    draft.showSubjectColors.value = false;
    draft.showSubject.value = true;
  }

  function toggleSubjectSort() {
    draft.subjectSortEnabled.value = !draft.subjectSortEnabled.value;
    if (draft.subjectSortEnabled.value)
      draft.subjectSortInput.value = String(nextSubjectSortOrder.value);
  }

  async function addSubject() {
    const sortText = draft.subjectSortInput.value.trim();
    if (draft.subjectSortEnabled.value && !/^[1-9]\d*$/.test(sortText)) {
      appStore.notifyTop("排序号请输入正整数");
      return;
    }
    const sortOrder = draft.subjectSortEnabled.value ? Number(sortText) : null;
    if (sortOrder !== null && !Number.isSafeInteger(sortOrder)) {
      appStore.notifyTop("排序号需为正整数");
      return;
    }
    try {
      if (draft.subjectEditId.value) {
        await api.updateSubject({
          id: draft.subjectEditId.value,
          name: draft.subjectName.value,
          color: draft.subjectColor.value,
          sortOrder,
          archived: false,
          deleted: false,
        });
      } else {
        await api.addSubject(
          draft.subjectName.value,
          draft.subjectColor.value,
          sortOrder,
        );
      }
      draft.subjectName.value = "";
      draft.subjectEditId.value = 0;
      draft.showSubject.value = false;
      await fetchAll();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function archiveSubject(subject: Subject) {
    try {
      await api.updateSubject({ ...subject, archived: true, deleted: false });
      await refreshProjectAndPlan();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function deleteSubject() {
    if (!draft.deleteSubjectTarget.value) return;
    try {
      await api.deleteSubject(draft.deleteSubjectTarget.value.id);
      draft.deleteSubjectTarget.value = null;
      await refreshProjectAndPlan();
    } catch (e) {
      appStore.setError(e);
    }
  }

  async function restoreSubject(subject: Subject) {
    try {
      await api.updateSubject({ ...subject, archived: false, deleted: false });
      await refreshProjectAndPlan();
    } catch (e) {
      appStore.setError(e);
    }
  }

  return {
    subjects,
    allSubjects,
    nextSubjectSortOrder,
    fetchAll,
    refreshProjectAndPlan,
    beginAddSubject,
    beginEditSubject,
    toggleSubjectSort,
    addSubject,
    archiveSubject,
    deleteSubject,
    restoreSubject,
  };
});
