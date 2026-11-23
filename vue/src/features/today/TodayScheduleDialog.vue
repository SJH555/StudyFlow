<script setup lang="ts">
import TimelineDialog from "../../shared/TimelineDialog.vue";
import TimelineRow from "../../shared/TimelineRow.vue";
import { storeToRefs } from "pinia";
import { usePlansStore } from "../../stores/plans";
import { usePlanDraft } from "../plans/usePlanDraft";
import {
  plannedStudyMinutes,
  planEnd,
  previewPlanNote,
} from "../../shared/format";

const { todayPlans, currentPlan } = storeToRefs(usePlansStore());
const { openScheduleNoteId } = usePlanDraft();
const showTodaySchedule = defineModel<boolean>({ default: false });
</script>

<template>
  <TimelineDialog
    v-model="showTodaySchedule"
    title="今日具体安排"
    :subtitle="`${todayPlans.length} 个计划时段 · 按开始时间排列`"
  >
    <div v-if="todayPlans.length" class="today-schedule-list">
      <TimelineRow
        v-for="item in todayPlans"
        :key="item.id"
        :color="item.color"
        :class="{
          'current-plan-row': currentPlan?.id === item.id,
          'has-note': !!item.note,
        }"
      >
        <template #time>
          <b>{{ item.startTime }}</b><span>{{ planEnd(item) }}</span>
        </template>
        <div class="plan-card-title">
          <b>{{ item.subject }}</b
          ><VBtn
            v-if="item.note"
            class="plan-note-chip"
            variant="flat"
            rounded="lg"
            size="x-small"
            :aria-expanded="openScheduleNoteId === item.id"
            @click.stop="
              openScheduleNoteId =
                openScheduleNoteId === item.id ? 0 : item.id
            "
            >备注 · {{ previewPlanNote(item.note, 24)
            }}<VTooltip
              activator="parent"
              :model-value="openScheduleNoteId === item.id"
              :open-on-hover="false"
              :open-on-focus="false"
              :open-on-click="false"
              location="top"
              transition="scale-transition"
              content-class="subject-tooltip today-note-tooltip"
              ><p>{{ item.note }}</p></VTooltip
            ></VBtn
          >
        </div>
        <small
          >时段 {{ item.duration }} 分钟 · 学习
          {{ plannedStudyMinutes(item) }} 分钟<span
            v-if="item.breakMinutes"
          >
            · 休息 {{ item.breakMinutes }} 分钟</span
          ></small
        >
        <template #trailing>
          <VChip
            v-if="currentPlan?.id === item.id"
            size="small"
            color="primary"
            variant="tonal"
          >当前时段</VChip>
        </template>
      </TimelineRow>
    </div>
    <div v-else class="today-schedule-empty">今天没有安排学习计划</div>
  </TimelineDialog>
</template>

<style scoped>
:global(.today-schedule-dialog .today-schedule-row.current-plan-row .plan-card) {
  border: 2px solid #2932e1;
  background: var(--surface-hidden);
  box-shadow: 0 4px 15px #2932e124;
}
:global(.today-schedule-row.current-plan-row .plan-time b) {
  color: #2932e1;
}

:global(.today-schedule-dialog .today-schedule-row .plan-note-chip.v-btn) {
  max-width: 320px;
  min-width: 0;
  height: 23px;
  padding-inline: 8px;
  border-radius: 8px !important;
  box-shadow: none !important;
  font-size: 11px;
  text-transform: none;
  letter-spacing: 0;
}

:global(.today-schedule-dialog .plan-note-chip .v-btn__content) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
