import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { api, type Subject } from "../api";
import { colorPalette } from "../shared/constants";
import { nextEffectiveDay } from "../shared/format";
import { useAppStore } from "./app";
import { usePlansStore } from "./plans";
import { useSubjectDraft } from "../features/subjects/useSubjectDraft";
import { usePlanDraft } from "../features/plans/usePlanDraft";

export function sameSubjectContent(left: Subject, right: Subject) {
  return left.name === right.name && left.color === right.color && left.sortOrder === right.sortOrder;
}

// 学习科目：科目数据的所有者，负责增删改、归档/恢复、排序号与科目计划联合刷新。
export const useSubjectsStore = defineStore("subjects", () => {
  const appStore = useAppStore();
  const plansStore = usePlansStore();
  const draft = useSubjectDraft();
  const planDraft = usePlanDraft();

  const allSubjects = ref<Subject[]>([]);
  const managedSubjects = ref<Subject[]>([]);
  const savingSubject = ref(false);
  const subjects = computed(() =>
    allSubjects.value.filter((subject) => !subject.archived && !subject.deleted),
  );

  const scheduledSubjects = computed(() =>
    managedSubjects.value.filter((subject) => !subject.archived && !subject.deleted),
  );

  // 下一个可用科目排序号（新增科目默认排最后）。
  const nextSubjectSortOrder = computed(
    () => Math.max(0, ...allSubjects.value.map((s) => s.sortOrder || 0)) + 1,
  );

  async function fetchAll() {
    allSubjects.value = await api.subjects(true, true);
  }

  // 科目与计划的联合刷新：并行请求，全部成功后才一起更新（不出现部分列表提前更新），
  // 并校正失效的项目筛选。计划的写入通过 plans store 的公开能力完成。
  async function refreshProjectAndPlan() {
    const [all, currentPlans, projectList, planList] = await Promise.all([
      api.subjects(true, true),
      api.plans(),
      api.subjects(true, true, true),
      api.managedPlans(),
    ]);
    allSubjects.value = all;
    managedSubjects.value = projectList;
    plansStore.managedPlans = planList;
    plansStore.setPlans(currentPlans);
    if (
      planDraft.planProjectFilter.value &&
      !planList.some((plan) => plan.subjectId === planDraft.planProjectFilter.value)
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
    if (savingSubject.value) return;
    const name = draft.subjectName.value.trim();
    if (!name || Array.from(name).length > 50) {
      appStore.notifyTop("学习项目名称需为 1 到 50 字");
      return;
    }
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

    const id = draft.subjectEditId.value;
    const color = draft.subjectColor.value;
    const scheduled = managedSubjects.value.find((subject) => subject.id === id);
    if (id && (!scheduled || scheduled.deleted)) {
      appStore.notifyTop("学习项目不存在或已删除");
      return;
    }

    const updated = scheduled ? { ...scheduled, name, color, sortOrder } : null;
    if (updated && scheduled && sameSubjectContent(scheduled, updated)) {
      draft.showSubject.value = false;
      draft.subjectEditId.value = 0;
      return;
    }

    savingSubject.value = true;
    try {
      if (updated) await api.updateSubject(updated);
      else await api.addSubject(name, color, sortOrder);
      draft.showSubject.value = false;
      draft.subjectName.value = "";
      draft.subjectEditId.value = 0;
      try {
        await refreshProjectAndPlan();
      } catch (e) {
        appStore.setError(`项目已保存，列表刷新失败。${e instanceof Error ? e.message : String(e)}`);
      }
    } catch (e) {
      appStore.setError(e);
    } finally {
      savingSubject.value = false;
    }
  }

  async function archiveSubject(subject: Subject) {
    try {
      await api.updateSubject({ ...subject, archived: true, deleted: false });
      await refreshProjectAndPlan();
      const today = allSubjects.value.find((item) => item.id === subject.id);
      appStore.notifyTop(today?.archived ? "已撤销启用" : "项目明日起停用，今天的安排保持原样");
    } catch (e) {
      appStore.setError(e);
    }
  }

  function beginDeleteSubject(subject: Subject) {
    if (savingSubject.value) return;
    draft.deleteSubjectDay.value = nextEffectiveDay();
    draft.deleteSubjectTarget.value = subject;
  }

  function cancelDeleteSubject() {
    if (!savingSubject.value) draft.deleteSubjectTarget.value = null;
  }

  async function deleteSubject() {
    const target = draft.deleteSubjectTarget.value;
    if (!target || savingSubject.value) return;
    savingSubject.value = true;
    try {
      const result = await api.deleteSubject(target.id, draft.deleteSubjectDay.value);
      if ("needsConfirmation" in result) {
        draft.deleteSubjectDay.value = result.effectiveDay;
        appStore.notifyTop("生效日期已更新，请确认新的日期");
        return;
      }
      draft.deleteSubjectTarget.value = null;
      await refreshProjectAndPlan();
    } catch (e) {
      appStore.setError(e);
    } finally {
      savingSubject.value = false;
    }
  }

  async function restoreSubject(subject: Subject) {
    try {
      await api.updateSubject({ ...subject, archived: false, deleted: false });
      await refreshProjectAndPlan();
      const today = allSubjects.value.find((item) => item.id === subject.id);
      appStore.notifyTop(today && !today.archived ? "已撤销停用" : "项目明日起启用");
    } catch (e) {
      appStore.setError(e);
    }
  }

  return {
    subjects,
    allSubjects,
    managedSubjects,
    scheduledSubjects,
    savingSubject,
    nextSubjectSortOrder,
    fetchAll,
    refreshProjectAndPlan,
    beginAddSubject,
    beginEditSubject,
    toggleSubjectSort,
    addSubject,
    archiveSubject,
    beginDeleteSubject,
    cancelDeleteSubject,
    deleteSubject,
    restoreSubject,
  };
});
