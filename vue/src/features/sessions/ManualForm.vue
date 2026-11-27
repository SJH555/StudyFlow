<script setup lang="ts">
import { ref, useId } from "vue";
import { formatDateChinese } from "../../shared/format";
import { useSessionsStore } from "../../stores/sessions";
import { useManualDraft } from "./useManualDraft";
import { hourOptions, minuteOptions } from "../../shared/constants";

const {
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
} = useManualDraft();

const subjectId = useId();
const titleId = useId();
const warningId = useId();
const dateMenuOpen = ref(false);
const { addManual, confirmManualPlanConflict, chooseManualDate } = useSessionsStore();
</script>

<template>
  <VDialog
    v-model="showManual"
    max-width="510"
    :persistent="manualSaving"
    :aria-labelledby="titleId"
    :aria-describedby="manualPlanConflict ? warningId : undefined"
  >
    <VCard class="manual-dialog-card" rounded="xl">
      <VCardTitle :id="titleId" class="subject-form-title">补充学习记录</VCardTitle>
      <p v-if="manualPlanConflict" :id="warningId" class="form-effective-notice" role="status">
        {{ manualPlanConflict.message }}
      </p>
      <VCardText class="manual-dialog-content">
        <div class="manual-dialog-field">
          <label :for="subjectId">学习项目</label>
          <VSelect
            :id="subjectId"
            v-model="manualSubject"
            :items="manualSubjects.map((s) => ({ title: s.name, value: s.id }))"
            :loading="manualSubjectsLoading"
            :disabled="manualSubjectsLoading || manualSaving"
            no-data-text="这天没有可用的学习项目"
            placeholder="选择学习项目"
            variant="outlined"
            density="comfortable"
            hide-details
          />
        </div>
        <div class="manual-dialog-field">
          <span>学习日期</span>
          <VMenu v-model="dateMenuOpen" :close-on-content-click="false">
            <template #activator="{ props: menuProps }">
              <VBtn
                v-bind="menuProps"
                class="manual-date-trigger"
                :aria-label="`学习日期，${formatDateChinese(manualDate)}`"
                :disabled="manualSaving"
                variant="outlined"
                prepend-icon="mdi-calendar-month-outline"
                append-icon="mdi-chevron-down"
              >{{ formatDateChinese(manualDate) }}</VBtn>
            </template>
            <VDatePicker
              :model-value="manualDate"
              color="primary"
              width="320"
              hide-title
              hide-header
              @update:model-value="chooseManualDate($event); dateMenuOpen = false"
            />
          </VMenu>
        </div>
        <div class="manual-dialog-field">
          <span>开始时间</span>
          <div class="plan-time-select">
            <VSelect
              v-model="manualStartHour"
              :disabled="manualSaving"
              :items="hourOptions"
              aria-label="补充记录开始小时"
              variant="outlined"
              density="comfortable"
              hide-details
            /><span>:</span
            ><VSelect
              v-model="manualStartMinute"
              :disabled="manualSaving"
              :items="minuteOptions"
              aria-label="补充记录开始分钟"
              variant="outlined"
              density="comfortable"
              hide-details
            />
          </div>
        </div>
        <div class="manual-dialog-field">
          <span>结束时间</span>
          <div class="plan-time-select">
            <VSelect
              v-model="manualEndHour"
              :disabled="manualSaving"
              :items="hourOptions"
              aria-label="补充记录结束小时"
              variant="outlined"
              density="comfortable"
              hide-details
            /><span>:</span
            ><VSelect
              v-model="manualEndMinute"
              :disabled="manualSaving"
              :items="minuteOptions"
              aria-label="补充记录结束分钟"
              variant="outlined"
              density="comfortable"
              hide-details
            />
          </div>
        </div>
        <div class="manual-duration-hint">
          学习时长 {{ Math.max(0, manualDurationMinutes) }} 分钟
          <p v-if="manualTimeError" class="manual-time-error" role="alert">
            {{ manualTimeError }}
          </p>
        </div>
      </VCardText>
      <div class="subject-form-actions">
        <VBtn variant="text" rounded="lg" :disabled="manualSaving" @click="showManual = false">
          取消
        </VBtn>
        <VBtn
          v-if="manualPlanConflict"
          variant="elevated"
          color="background"
          elevation="1"
          rounded="lg"
          :disabled="manualSaving"
          :loading="manualSaving"
          @click="confirmManualPlanConflict"
        >仍然补录</VBtn>
        <VBtn
          v-else
          variant="elevated"
          color="background"
          elevation="1"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          :loading="manualSaving"
          :disabled="manualSaving || manualSubjectsLoading || !manualSubjects.length || !manualSubject || !!manualTimeError"
          @click="addManual"
          >保存记录</VBtn
        >
      </div></VCard
    ></VDialog
  >
</template>

<style scoped>
.manual-duration-hint {
  color: var(--muted);
  font-size: 12px;
}
.manual-time-error {
  color: rgb(var(--v-theme-error));
  margin: 6px 0 0;
}
.manual-date-trigger.v-btn {
  width: 100%;
  height: 48px;
  grid-template-columns: max-content minmax(0, 1fr) max-content;
  padding-inline: 16px;
  border-color: rgba(var(--v-theme-on-surface), 0.38);
  border-radius: 10px;
  font-size: 14px;
  font-weight: 400;
  letter-spacing: normal;
}

.manual-date-trigger :deep(.v-btn__content) {
  justify-content: flex-start;
}
.manual-dialog-field .plan-time-select {
  display: flex;
  align-items: center;
  gap: 8px;
}
.manual-dialog-card > .subject-form-actions {
  margin-top: 10px;
}

.manual-dialog-card {
  display: flex;
  flex-direction: column;
  background: var(--surface-card);
  color: var(--ink);
}
.manual-dialog-content {
  display: grid;
  gap: 16px;
  padding: 12px 24px 8px;
}
.manual-dialog-field {
  display: grid;
  gap: 8px;
  color: var(--ink);
  font-size: 14px;
  font-weight: 600;
}
.manual-dialog-content :deep(.v-field) {
  border-radius: 10px;
  background: var(--surface-card);
}
</style>
