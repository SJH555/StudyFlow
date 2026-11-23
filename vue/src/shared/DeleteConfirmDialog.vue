<script setup lang="ts">
import { useId } from "vue";
import ListActionButton from "./ListActionButton.vue";

defineProps<{
  open: boolean;
  title: string;
  notice: string;
  busy?: boolean;
}>();
const emit = defineEmits<{ cancel: []; confirm: [] }>();

const titleId = useId();
const noticeId = useId();

function confirm(event: MouseEvent) {
  if (event.detail < 2) emit("confirm");
}
</script>

<template>
  <VDialog
    :model-value="open"
    max-width="460"
    :persistent="busy"
    :aria-labelledby="titleId"
    :aria-describedby="noticeId"
    @update:model-value="!$event && $emit('cancel')"
  >
    <VCard class="delete-confirm-dialog plan-confirm-dialog plan-dialog-card" rounded="xl">
      <VCardTitle :id="titleId" class="plan-dialog-title">{{ title }}</VCardTitle>
      <p :id="noticeId" class="form-effective-notice" aria-live="polite">{{ notice }}</p>
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn variant="text" rounded="lg" :disabled="busy" @click="$emit('cancel')">取消</VBtn>
        <ListActionButton
          action="delete"
          :loading="busy"
          :disabled="busy"
          @click="confirm"
          @keydown.enter="$event.repeat && $event.preventDefault()"
        />
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.delete-confirm-dialog .plan-dialog-title {
  white-space: normal;
  overflow-wrap: anywhere;
}
</style>
