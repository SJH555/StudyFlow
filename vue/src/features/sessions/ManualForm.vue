<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useSubjectsStore } from "../../stores/subjects";
import { useSessionsStore } from "../../stores/sessions";
import { useManualDraft } from "./useManualDraft";
import { hourOptions, minuteOptions } from "../../shared/constants";

const {
  manualSubject,
  manualStartHour,
  manualStartMinute,
  manualEndHour,
  manualEndMinute,
  showManual,
  manualDurationMinutes,
  manualTimeError,
} = useManualDraft();

const { subjects } = storeToRefs(useSubjectsStore());
const { addManual } = useSessionsStore();
</script>

<template>
  <VDialog v-model="showManual" max-width="510"
    ><VCard class="manual-dialog-card" rounded="xl"
      ><VCardTitle class="subject-form-title">补充学习记录</VCardTitle
      ><VCardText class="manual-dialog-content">
        <VSelect
          v-model="manualSubject"
          :items="subjects.map((s) => ({ title: s.name, value: s.id }))"
          label="学习项目"
          variant="outlined"
          density="comfortable"
          hide-details="auto"
        />
        <div class="manual-dialog-time">
          <span>开始时间</span>
          <div class="plan-time-select">
            <VSelect
              v-model="manualStartHour"
              :items="hourOptions"
              aria-label="补充记录开始小时"
              variant="outlined"
              density="comfortable"
              hide-details
            /><span>:</span
            ><VSelect
              v-model="manualStartMinute"
              :items="minuteOptions"
              aria-label="补充记录开始分钟"
              variant="outlined"
              density="comfortable"
              hide-details
            />
          </div>
        </div>
        <div class="manual-dialog-time">
          <span>结束时间</span>
          <div class="plan-time-select">
            <VSelect
              v-model="manualEndHour"
              :items="hourOptions"
              aria-label="补充记录结束小时"
              variant="outlined"
              density="comfortable"
              hide-details
            /><span>:</span
            ><VSelect
              v-model="manualEndMinute"
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
        <VBtn variant="text" rounded="lg" @click="showManual = false">取消</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          :disabled="!subjects.length || !manualSubject || !!manualTimeError"
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
.manual-dialog-time .plan-time-select {
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
  gap: 18px;
  padding: 12px 24px 8px;
}
.manual-dialog-time {
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
