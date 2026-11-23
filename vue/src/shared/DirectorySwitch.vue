<script setup lang="ts" generic="T extends string">
defineProps<{
  label: string;
  options: { value: T; label: string; count?: number }[];
}>();

const selected = defineModel<T>({ required: true });
</script>

<template>
  <div class="directory-switch">
    <VBtnToggle
      v-model="selected"
      mandatory
      color="primary"
      variant="tonal"
      density="comfortable"
      role="group"
      :aria-label="label"
    >
      <VBtn
        v-for="option in options"
        :key="option.value"
        :value="option.value"
        :aria-pressed="selected === option.value"
      >
        {{ option.label }}
        <span v-if="option.count !== undefined" class="directory-count">{{ option.count }}</span>
      </VBtn>
    </VBtnToggle>
  </div>
</template>

<style scoped>
.directory-switch {
  margin-bottom: var(--space-4);
}
.directory-switch :deep(.v-btn-toggle) {
  border: 1px solid var(--outline-card);
  border-radius: 9px;
  overflow: hidden;
}
.directory-switch :deep(.v-btn) {
  min-width: 110px;
  text-transform: none;
}
.directory-count {
  display: inline-grid;
  place-items: center;
  min-width: 21px;
  height: 21px;
  margin-inline-start: var(--space-2);
  padding-inline: 5px;
  border-radius: 11px;
  background: var(--surface-card);
  font-size: var(--text-small);
}
</style>
