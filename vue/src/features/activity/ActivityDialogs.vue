<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useActivityStore } from "../../stores/activity";

const activityStore = useActivityStore();
const { showActivity, activityText, activitySaving, reminderActive } =
  storeToRefs(activityStore);
const { dismissActivityReminder, addActivity } = activityStore;
</script>

<template>
  <VDialog
    v-if="!reminderActive"
    v-model="showActivity"
    :persistent="activitySaving"
    max-width="510"
    ><VCard class="activity-entry-dialog" rounded="xl"
      ><VCardTitle class="subject-form-title">记录当前活动</VCardTitle
      ><VCardText class="activity-entry-content"
        ><VTextarea
          v-model="activityText"
          :readonly="activitySaving"
          label="当前活动"
          placeholder="填写正在进行的活动"
          variant="outlined"
          density="comfortable"
          rows="3"
          hide-details="auto"
          autofocus
      /></VCardText>
      <div class="subject-form-actions">
        <VBtn
          variant="text"
          rounded="lg"
          :disabled="activitySaving"
          @click="showActivity = false"
          >取消</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          :loading="activitySaving"
          :disabled="activitySaving"
          @click="addActivity"
          >保存记录</VBtn
        >
      </div></VCard
    ></VDialog
  >
  <VDialog
    :model-value="reminderActive"
    :persistent="activitySaving"
    max-width="510"
    :z-index="2400"
    @update:model-value="!$event && dismissActivityReminder()"
    ><VCard v-if="reminderActive" class="activity-entry-dialog" rounded="xl"
      ><VCardTitle class="subject-form-title"
        >活动提醒 · 记录当前活动</VCardTitle
      ><VCardText class="activity-entry-content"
        ><VTextarea
          v-model="activityText"
          :readonly="activitySaving"
          label="当前活动"
          placeholder="填写正在进行的活动"
          variant="outlined"
          density="comfortable"
          rows="3"
          hide-details="auto"
          autofocus
      /></VCardText>
      <div class="subject-form-actions">
        <VBtn
          variant="text"
          rounded="lg"
          :disabled="activitySaving"
          @click="dismissActivityReminder"
          >关闭</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          :loading="activitySaving"
          :disabled="activitySaving"
          @click="addActivity"
          >提交记录并停止铃声</VBtn
        >
      </div></VCard
    ></VDialog
  >
</template>

<style scoped>
.activity-entry-dialog > .subject-form-actions {
  margin-top: 10px;
}
.activity-entry-dialog {
  display: flex;
  flex-direction: column;
  background: var(--surface-card);
  color: var(--ink);
}
.activity-entry-content {
  padding: 12px 24px 8px;
}
.activity-entry-content :deep(.v-field) {
  border-radius: 10px;
  background: var(--surface-card);
}
</style>
