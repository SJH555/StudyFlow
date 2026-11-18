<script setup lang="ts">
defineProps<{ source: string }>();
</script>

<template>
  <span class="session-source-badge" :class="{ 'is-manual': source === 'manual' }">
    <VIcon :icon="source === 'manual' ? 'mdi-pencil-outline' : 'mdi-timer-outline'" size="12" />
    {{ source === "manual" ? "手动补录" : "计时学习" }}
  </span>
</template>

<style scoped>
.session-source-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 5px;
  background: rgba(var(--v-theme-primary), 0.08);
  color: rgb(var(--v-theme-primary));
  font-size: 11px;
  font-weight: 400;
  line-height: 1.6;
  white-space: nowrap;
  vertical-align: middle;
}
.session-source-badge.is-manual {
  background: #fcf3e5;
  color: #8a6021;
}
</style>
