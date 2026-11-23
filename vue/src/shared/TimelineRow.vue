<script setup lang="ts">
defineProps<{ color: string; as?: "li" }>();
</script>

<template>
  <component :is="as ?? 'div'" class="plan-row today-schedule-row">
    <div class="plan-time"><slot name="time" /></div>
    <div class="timeline" aria-hidden="true">
      <div class="timeline-line"></div>
      <div class="timeline-dot" :style="{ background: color }"></div>
    </div>
    <div class="plan-card">
      <div class="plan-card-main">
        <span class="subject-dot" :style="{ background: color }" aria-hidden="true"></span>
        <div class="plan-card-copy"><slot /></div>
      </div>
      <slot name="trailing" />
    </div>
  </component>
</template>

<style scoped>
:global(.today-schedule-row.plan-row) {
  height: auto;
  min-height: 62px;
  grid-template-columns: 76px 24px minmax(0, 1fr);
  padding: 5px 20px;
}

:global(.today-schedule-row .plan-card) {
  width: 100%;
  background: #fff;
}

:global(.today-schedule-row .plan-card-copy) {
  min-width: 0;
  overflow: hidden;
}

:global(.today-schedule-row .plan-card-copy small) {
  display: block;
  margin-top: 3px;
  font-size: 11px;
  line-height: 1.45;
  white-space: normal;
}

:global(.today-schedule-row .plan-note) {
  margin-top: 4px;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.45;
  white-space: normal;
  overflow-wrap: anywhere;
}

:global(.today-schedule-row .timeline-line) {
  top: 0 !important;
  bottom: 0 !important;
}

:global(.today-schedule-row .timeline-dot) {
  outline-color: #fff;
}

:global(.today-schedule-row .plan-card) {
  height: auto;
  min-height: 50px;
  margin: 0;
  padding: 8px 12px;
}

:global(.today-schedule-row .plan-time) {
  padding-top: 7px;
  gap: 2px;
}

:global(.today-schedule-row .plan-card-copy small) {
  margin-top: 2px;
  font-size: 11px;
}

:global(.today-schedule-row .timeline-dot) {
  top: 12px;
}
</style>
