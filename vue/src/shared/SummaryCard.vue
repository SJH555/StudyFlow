<script setup lang="ts">
defineProps<{
  label: string;
  icon: string;
  tone: "primary" | "warning" | "success";
  tall?: boolean;
}>();
</script>

<template>
  <VCard
    tag="article"
    variant="flat"
    class="summary-card"
    :class="[`summary-card--${tone}`, { 'summary-card--tall': tall }]"
  >
    <div class="summary-card-content">
      <span>{{ label }}</span>
      <strong><slot /></strong>
      <small v-if="$slots.detail"><slot name="detail" /></small>
    </div>
    <span class="summary-card-icon" aria-hidden="true"><VIcon :icon="icon" /></span>
  </VCard>
</template>

<style scoped>
.summary-card.v-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--outline-card);
  border-radius: var(--shape-medium);
  background: var(--surface-app);
  box-shadow: var(--elevation-item);
}

.summary-card--tall.v-card {
  min-block-size: calc(var(--size-field) * 2 + var(--space-5));
  padding: var(--space-4) var(--space-5);
  border-color: var(--summary-accent);
  box-shadow:
    var(--elevation-item),
    0 4px 14px color-mix(in oklab, var(--summary-accent) 18%, transparent);
}

.summary-card-content {
  display: grid;
  gap: var(--space-2);
  min-inline-size: 0;
}

.summary-card-content > span,
.summary-card-content > small {
  color: var(--muted);
  font-size: var(--text-small);
}

.summary-card-content strong {
  color: var(--ink);
  font-size: var(--text-metric);
  line-height: 1;
}

.summary-card-content strong :deep(em) {
  margin-inline-start: 2px;
  font-size: var(--text-body);
  font-style: normal;
}

.summary-card-icon {
  display: grid;
  place-items: center;
  flex: none;
  inline-size: var(--size-list-action);
  block-size: var(--size-list-action);
  border-radius: var(--shape-medium);
  color: var(--summary-accent);
  background: color-mix(in srgb, var(--summary-accent) 12%, var(--surface-card));
}

.summary-card-icon .v-icon {
  font-size: var(--text-heading);
}

.summary-card--primary {
  --summary-accent: var(--review-primary, rgb(var(--v-theme-primary)));
}

.summary-card--warning {
  --summary-accent: rgb(var(--v-theme-warning));
}

.summary-card--success {
  --summary-accent: rgb(var(--v-theme-success));
}
</style>
