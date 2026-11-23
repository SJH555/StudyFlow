<script setup lang="ts">
defineProps<{ enabled: boolean; undo?: boolean }>();
</script>

<template>
  <VBtn
    :prepend-icon="undo ? 'mdi-undo' : enabled ? 'mdi-eye-off-outline' : 'mdi-eye-outline'"
    variant="flat"
    color="directory-status"
    size="small"
    rounded="lg"
  >
    {{ undo ? "撤销" : enabled ? "停用" : "启用" }}
  </VBtn>
</template>
