<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { Goal } from "./types";
import { dateKey, formatDateChinese, parseDateKey } from "../../shared/format";
import { useGoalsStore } from "../../stores/goals";
import { useAppStore } from "../../stores/app";

const props = defineProps<{
  modelValue: boolean;
  goal: Goal | null;
}>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [];
}>();

const goalsStore = useGoalsStore();
const appStore = useAppStore();
const title = ref("");
const deadline = ref(dateKey(new Date()));
const note = ref("");
const saving = ref(false);
const dateMenuOpen = ref(false);
const errorMessage = ref("");
const dialogOpen = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});
const canSave = computed(
  () => title.value.trim().length > 0 && Boolean(deadline.value) && !saving.value,
);

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    dateMenuOpen.value = false;
    title.value = props.goal?.title || "";
    deadline.value = props.goal?.deadline || dateKey(new Date());
    note.value = props.goal?.note || "";
    errorMessage.value = "";
  },
);

function pickDeadline(value: unknown) {
  const day = parseDateKey(value);
  if (!day) return;
  deadline.value = day;
  dateMenuOpen.value = false;
}

async function save() {
  if (!canSave.value) return;
  saving.value = true;
  errorMessage.value = "";
  try {
    await goalsStore.save({
      id: props.goal?.id,
      title: title.value,
      deadline: deadline.value,
      note: note.value,
    });
    emit("saved");
    dialogOpen.value = false;
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : String(error);
    appStore.setError(error);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <VDialog v-model="dialogOpen" max-width="520">
    <VCard class="goal-dialog plan-dialog-card" rounded="xl">
      <VCardTitle class="plan-dialog-title">
        {{ goal ? "编辑阶段目标" : "新建阶段目标" }}
      </VCardTitle>
      <VCardText class="plan-dialog-content">
        <VTextField
          v-model="title"
          class="form-name-input"
          label=""
          aria-label="目标"
          placeholder="请输入目标"
          variant="solo-filled"
          density="comfortable"
          persistent-placeholder
          maxlength="50"
          hide-details="auto"
          autofocus
        />
        <div class="plan-dialog-field">
          <span>截止日期</span>
          <VMenu v-model="dateMenuOpen" :close-on-content-click="false">
            <template #activator="{ props: menuProps }">
              <VBtn
                v-bind="menuProps"
                class="goal-date-trigger"
                variant="outlined"
                rounded="lg"
                prepend-icon="mdi-calendar-month-outline"
                append-icon="mdi-chevron-down"
              >
                {{ formatDateChinese(deadline) }}
              </VBtn>
            </template>
            <VDatePicker
              :model-value="deadline"
              color="primary"
              width="320"
              hide-title
              hide-header
              @update:model-value="pickDeadline"
            />
          </VMenu>
          <small class="goal-deadline-hint">截止时间为所选日期的 24:00（即次日 00:00）。</small>
        </div>
        <VTextarea
          v-model="note"
          label="详情（最多 500 字）"
          placeholder="补充目标范围或完成标准（选填）"
          variant="outlined"
          density="comfortable"
          rows="3"
          maxlength="500"
          hide-details="auto"
        /><small class="plan-note-count">{{ Array.from(note).length }} / 500</small>
        <p v-if="errorMessage" class="goal-dialog-error" role="alert">
          {{ errorMessage }}
        </p>
      </VCardText>
      <div class="subject-form-actions">
        <VBtn variant="text" rounded="lg" @click="dialogOpen = false">取消</VBtn>
        <VBtn
          color="primary"
          variant="tonal"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          :disabled="!canSave"
          :loading="saving"
          @click="save"
        >
          {{ goal ? "保存修改" : "保存目标" }}
        </VBtn>
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.goal-dialog .plan-dialog-content {
  gap: 20px;
}

.goal-dialog .plan-dialog-field {
  gap: 10px;
}

.goal-date-trigger {
  justify-content: space-between;
  justify-self: start;
  inline-size: 220px;
  max-inline-size: 100%;
  block-size: var(--size-field);
  background: var(--surface-card);
  border-color: var(--outline-card);
  color: var(--ink);
  text-transform: none;
  letter-spacing: 0;
}

.goal-deadline-hint {
  color: var(--muted);
  font-size: var(--text-small);
  font-weight: 400;
}

.goal-dialog-error {
  margin: 0;
  color: rgb(var(--v-theme-error));
  font-size: var(--text-small);
}
</style>
