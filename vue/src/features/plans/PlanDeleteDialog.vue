<script setup lang="ts">
import { computed } from "vue";
import DeleteConfirmDialog from "../../shared/DeleteConfirmDialog.vue";
import { formatDateChinese, planEnd } from "../../shared/format";
import { usePlanDraft } from "./usePlanDraft";
import { usePlansForm } from "./usePlans";

const { deletePlanTarget, deletePlanDay, savingPlan } = usePlanDraft();
const { cancelDeletePlan, confirmDeletePlan } = usePlansForm();
const notice = computed(() => {
  const plan = deletePlanTarget.value;
  if (!plan) return "";
  return `${plan.startTime}–${planEnd(plan)} · ${formatDateChinese(deletePlanDay.value)}起停止排程。`;
});
</script>

<template>
  <DeleteConfirmDialog
    :open="!!deletePlanTarget"
    :title="`删除「${deletePlanTarget?.subject ?? ''}」的计划？`"
    :notice="notice"
    :busy="savingPlan"
    @cancel="cancelDeletePlan"
    @confirm="confirmDeletePlan"
  />
</template>
