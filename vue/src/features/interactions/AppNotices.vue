<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { useWindowFrame } from "../window/useWindowFrame";

const appStore = useAppStore();
const { topNotice, noticeId, noticeTone, focusMode } = storeToRefs(appStore);
const { windowsTitlebar, windowsMaximised } = useWindowFrame(appStore.setError);

const icons = {
  info: "mdi-information-outline",
  success: "mdi-check-circle-outline",
  error: "mdi-alert-circle-outline",
} as const;

const dismiss = computed(() => {
  const id = noticeId.value;
  return (visible: boolean) => {
    if (!visible) appStore.clearTopNotice(id);
  };
});
</script>

<template>
  <VSnackbar
    :key="noticeId"
    :model-value="!!topNotice"
    :class="[
      'app-top-snackbar',
      windowsTitlebar && !focusMode ? 'windows-top-snackbar' : '',
      windowsMaximised ? 'windows-top-snackbar-maximised' : '',
    ]"
    location="top"
    transition="slide-y-transition"
    :timeout="noticeTone === 'error' ? 5000 : 4500"
    @update:model-value="dismiss"
  >
    <VIcon :icon="icons[noticeTone]" size="small" class="app-top-snackbar-icon" aria-hidden="true" />
    <span>{{ topNotice }}</span>
  </VSnackbar>
</template>

<style scoped>
:global(.app-top-snackbar .v-snackbar__wrapper) {
  min-width: 0;
  max-width: min(420px, calc(100vw - var(--space-4) * 2));
  background: var(--surface-app);
  color: var(--ink);
  border: 1px solid var(--outline-notice);
  border-radius: var(--shape-medium);
  box-shadow: var(--elevation-notice);
}

:global(.app-top-snackbar .v-snackbar__content) {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
}

:global(.app-top-snackbar-icon) {
  flex: none;
  color: color-mix(in srgb, var(--ink) 80%, var(--surface-app));
}

@media (prefers-reduced-motion: reduce) {
  :global(.app-top-snackbar .v-snackbar__wrapper) {
    transition: none;
  }
}
</style>
