<script setup lang="ts">
import { usePlanDraft } from "./usePlanDraft";
import { usePlansForm } from "./usePlans";
const { pendingPlanAction, pendingPlan } = usePlanDraft();
const { cancelPlanAction, confirmPlanAction } = usePlansForm();
</script>

<template>
  <VDialog
    :model-value="!!(pendingPlanAction && pendingPlan)"
    max-width="460"
    @update:model-value="!$event && cancelPlanAction()"
  >
    <VCard
      v-if="pendingPlan"
      class="plan-confirm-dialog plan-dialog-card"
      rounded="xl"
    >
      <VCardTitle class="plan-dialog-title">{{
        pendingPlanAction === "create"
          ? "确认新建学习计划？"
          : pendingPlanAction === "edit"
            ? "确认修改学习计划？"
            : "确认删除学习计划？"
      }}</VCardTitle>
      <VCardText class="plan-confirm-content">
        <p>
          {{ pendingPlan.subject }} · {{ pendingPlan.startTime }} 开始 ·
          {{ pendingPlan.duration }} 分钟时段<span
            v-if="pendingPlan.breakMinutes"
          >
            · 休息 {{ pendingPlan.breakMinutes }} 分钟</span
          >
        </p>
        <p
          v-if="pendingPlan.note && pendingPlanAction !== 'delete'"
          class="confirm-plan-note"
        >
          备注：{{ pendingPlan.note }}
        </p>
      </VCardText>
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn variant="text" rounded="lg" @click="cancelPlanAction">取消</VBtn
        ><VBtn
          variant="tonal"
          :color="pendingPlanAction === 'delete' ? 'error' : 'primary'"
          rounded="lg"
          @click="confirmPlanAction"
          >{{ pendingPlanAction === "delete" ? "确认删除" : "确认保存" }}</VBtn
        >
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.confirm-plan-note {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.plan-confirm-dialog .plan-confirm-content .confirm-plan-note {
  margin-top: 12px;
  color: var(--muted);
}
</style>
