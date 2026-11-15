<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  action: "accept" | "complete" | "reopen" | "edit" | "delete";
}>();

const actions = {
  accept: { label: "验收", icon: "mdi-check", color: "primary" },
  complete: { label: "完成", icon: "mdi-check", color: "primary" },
  reopen: { label: "重新开启", icon: "mdi-undo", color: "primary" },
  edit: { label: "编辑", icon: "mdi-pencil-outline", color: "primary" },
  delete: { label: "删除", icon: "mdi-delete-outline", color: "error" },
};
const appearance = computed(() => actions[props.action]);
</script>

<template>
  <VBtn
    class="list-action-button"
    variant="tonal"
    :color="appearance.color"
    size="small"
    rounded="lg"
    :prepend-icon="appearance.icon"
  >
    {{ appearance.label }}
  </VBtn>
</template>

<style scoped>
.list-action-button.v-btn {
  min-inline-size: var(--size-list-action-width);
  block-size: var(--size-list-action);
  padding-inline: var(--space-2);
  font-size: var(--text-small);
  font-weight: var(--weight-medium);
  text-transform: none;
  letter-spacing: 0;
}
.list-action-button :deep(.v-btn__prepend) {
  margin-inline-end: var(--space-1);
}
@media (max-width: 1050px) {
  :global(.subject-directory-actions) .list-action-button {
    min-inline-size: 0;
  }
}
</style>
