<script setup lang="ts">
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
  <VDialog
    v-model="showTodaySchedule"
    :z-index="10"
    max-width="1000"
    aria-label="今日具体安排"
  >
    <VCard class="today-schedule-dialog" rounded="xl">
      <div class="today-schedule-header">
        <div>
          <h2>今日具体安排</h2>
          <p>{{ todayPlans.length }} 个计划时段 · 按开始时间排列</p>
        </div>
        <VBtn
          icon="mdi-close"
          variant="text"
          aria-label="关闭今日具体安排"
          @click="showTodaySchedule = false"
        />
      </div>
      <VDivider />
      <div v-if="todayPlans.length" class="today-schedule-list">
        <div
          v-for="item in todayPlans"
          :key="item.id"
          class="plan-row today-schedule-row"
          :class="{
            'current-plan-row': currentPlan?.id === item.id,
            'has-note': !!item.note,
          }"
        >
          <div class="plan-time">
            <b>{{ item.startTime }}</b
            ><span>{{ planEnd(item) }}</span>
          </div>
          <div class="timeline">
            <div class="timeline-line"></div>
            <div class="timeline-dot" :style="{ background: item.color }"></div>
          </div>
          <div class="plan-card">
            <div class="plan-card-main">
              <div
                class="subject-dot"
                :style="{ background: item.color }"
              ></div>
              <div class="plan-card-copy">
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
              </div>
            </div>
            <VChip
              v-if="currentPlan?.id === item.id"
              size="small"
              color="primary"
              variant="tonal"
              >当前时段</VChip
            >
          </div>
        </div>
      </div>
      <div v-else class="today-schedule-empty">今天没有安排学习计划</div>
    </VCard>
  </VDialog>
</template>

<style scoped>
:global(.today-schedule-row.current-plan-row .plan-card) {
  border: 2px solid #2932e1;
  background: #f3f6ff;
  box-shadow: 0 4px 15px #2932e124;
}
:global(.today-schedule-row.current-plan-row .plan-time b) {
  color: #2932e1;
}

/* 今日安排弹窗专属样式。 */

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

:global(.today-schedule-row.current-plan-row .plan-card) {
  background: var(--surface-hidden);
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

:global(.today-schedule-empty) {
  display: grid;
  place-items: center;
  min-height: 240px;
  color: var(--muted);
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

:global(.today-schedule-dialog .today-schedule-header) {
  padding: 24px 24px 16px;
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
