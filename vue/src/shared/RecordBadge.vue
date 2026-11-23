<script setup lang="ts">
defineProps<{ icon: string }>();
</script>

<template>
  <span class="record-badge">
    <VIcon :icon="icon" size="12" />
    <slot />
  </span>
</template>

<style scoped>
.record-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 7px;
  border-radius: 5px;
  background: rgb(var(--v-theme-directory-status));
  color: rgb(var(--v-theme-on-directory-status));
  font-size: 11px;
  font-weight: 400;
  line-height: 1.6;
  white-space: nowrap;
  vertical-align: middle;
}
</style>
