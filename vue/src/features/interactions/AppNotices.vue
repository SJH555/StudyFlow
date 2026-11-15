<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { useWindowFrame } from "../window/useWindowFrame";

// 应用级提示：顶部通知 snackbar 与全局错误 toast。
const appStore = useAppStore();
const { topNotice, error, focusMode } = storeToRefs(appStore);
const { windowsTitlebar, windowsMaximised } = useWindowFrame(
  appStore.setError,
);

const topNoticeVisible = computed({
  get: () => !!topNotice.value,
  set: (visible: boolean) => {
    if (!visible) appStore.clearTopNotice();
  },
});
</script>

<template>
  <VSnackbar
    v-model="topNoticeVisible"
    :class="[
      'review-save-snackbar is-success app-top-snackbar',
      windowsTitlebar && !focusMode ? 'windows-top-snackbar' : '',
      windowsMaximised ? 'windows-top-snackbar-maximised' : '',
    ]"
    location="top"
    transition="slide-y-transition"
    :timeout="4500"
    role="status"
    ><VIcon
      icon="mdi-information-outline"
      size="small"
      class="review-save-snackbar-icon"
    />{{ topNotice }}</VSnackbar
  >
  <div v-if="error" class="toast">{{ error }}</div>
</template>

<style scoped>
.toast {
  position: fixed;
  right: 24px;
  bottom: 24px;
  background: #393746;
  color: white;
  padding: 12px 16px;
  border-radius: 8px;
  font-size: 11px;
  z-index: 10;
  box-shadow: 0 8px 25px #0002;
  max-width: 400px;
}
.toast {
  font-size: 13px;
  padding: 14px 18px;
  border-radius: 10px;
  max-width: 520px;
}
.toast {
  z-index: 120;
}
</style>
