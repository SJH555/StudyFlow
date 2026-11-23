<script setup lang="ts">
import { useId } from "vue";

defineProps<{ title: string; subtitle: string }>();
const open = defineModel<boolean>({ default: false });
const titleId = useId();
</script>

<template>
  <VDialog
    v-model="open"
    :z-index="10"
    max-width="1000"
    :aria-labelledby="titleId"
  >
    <VCard class="today-schedule-dialog" rounded="xl">
      <div class="today-schedule-header">
        <div>
          <h2 :id="titleId">{{ title }}</h2>
          <p>{{ subtitle }}</p>
        </div>
        <VBtn
          icon="mdi-close"
          variant="text"
          :aria-label="`关闭${title}`"
          @click="open = false"
        />
      </div>
      <VDivider />
      <slot />
    </VCard>
  </VDialog>
</template>

<style scoped>
:global(.today-schedule-dialog.v-card) {
  background: #fff;
  color: var(--ink);
  border: 1px solid var(--outline-card);
  box-shadow: 0 18px 50px color-mix(in srgb, var(--ink) 18%, transparent);
}

:global(.today-schedule-header) {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 21px 30px 17px;
}

:global(.today-schedule-header h2) {
  margin: 0;
  font-size: 21px;
  line-height: 1.4;
}

:global(.today-schedule-header p) {
  margin: 5px 0 0;
  color: var(--muted);
  font-size: 13px;
}

:global(.today-schedule-list) {
  height: min(70vh, 680px);
  overflow-y: auto;
  padding: 0 10px 10px;
}

:global(.today-schedule-empty) {
  display: grid;
  place-items: center;
  min-height: 240px;
  color: var(--muted);
}

:global(.today-schedule-dialog .today-schedule-header) {
  padding: 24px 24px 16px;
}
</style>
