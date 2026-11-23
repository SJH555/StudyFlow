<script setup lang="ts">
import RecordBadge from "../../shared/RecordBadge.vue";

defineProps<{ source: string }>();
</script>

<template>
  <RecordBadge
    :icon="source === 'manual' ? 'mdi-pencil-outline' : 'mdi-timer-outline'"
  >
    {{ source === "manual" ? "手动补录" : "计时学习" }}
  </RecordBadge>
</template>
