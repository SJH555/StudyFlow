<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import DeleteConfirmDialog from "../../shared/DeleteConfirmDialog.vue";
import { formatDateChinese } from "../../shared/format";
import { useSubjectDraft } from "./useSubjectDraft";
import { useSubjectsStore } from "../../stores/subjects";

const { deleteSubjectTarget, deleteSubjectDay } = useSubjectDraft();
const subjectsStore = useSubjectsStore();
const { savingSubject } = storeToRefs(subjectsStore);
const { cancelDeleteSubject, deleteSubject } = subjectsStore;
const notice = computed(() => deleteSubjectDay.value
  ? `${formatDateChinese(deleteSubjectDay.value)}起移出列表并停止相关排程，历史记录保留。`
  : "");
</script>

<template>
  <DeleteConfirmDialog
    :open="!!deleteSubjectTarget"
    :title="`删除“${deleteSubjectTarget?.name ?? ''}”？`"
    :notice="notice"
    :busy="savingSubject"
    @cancel="cancelDeleteSubject"
    @confirm="deleteSubject"
  />
</template>
