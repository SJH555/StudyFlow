<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useSubjectDraft } from "./useSubjectDraft";
import { usePlansStore } from "../../stores/plans";
import {
  planEnd,
  previewPlanNote,
  plannedStudyMinutes,
} from "../../shared/format";
import { weekNames } from "../../shared/constants";
const {
  subjectPlanTarget,
  showSubjectPlans,
  openSubjectPlanNoteId,
} = useSubjectDraft();
const { plans } = storeToRefs(usePlansStore());
</script>

<template>
  <VDialog v-model="showSubjectPlans" max-width="780" aria-label="关联学习计划"
    ><VCard class="subject-plans-dialog" rounded="lg"
      ><div class="today-schedule-header">
        <div>
          <h2>{{ subjectPlanTarget?.name }} · 关联计划</h2>
          <p>
            {{
              plans.filter((p) => p.subjectId === subjectPlanTarget?.id).length
            }}
            个计划时段 · 按开始时间排列
          </p>
        </div>
        <VBtn
          icon="mdi-close"
          variant="text"
          aria-label="关闭关联计划"
          @click="showSubjectPlans = false"
        />
      </div>
      <VDivider />
      <div class="subject-plans-list">
        <div
          v-for="p in plans
            .filter((p) => p.subjectId === subjectPlanTarget?.id)
            .slice()
            .sort((a, b) => a.startTime.localeCompare(b.startTime))"
          :key="p.id"
          class="plan-row today-schedule-row"
        >
          <div class="plan-time">
            <b>{{ p.startTime }}</b
            ><span>{{ planEnd(p) }}</span>
          </div>
          <div class="timeline">
            <div class="timeline-line"></div>
            <div class="timeline-dot" :style="{ background: p.color }"></div>
          </div>
          <div class="plan-card">
            <div class="plan-card-main">
              <span class="subject-dot" :style="{ background: p.color }"></span>
              <div class="plan-card-copy">
                <div class="plan-card-title">
                  <b>{{ p.subject }}</b
                  ><VBtn
                    v-if="p.note"
                    class="plan-note-chip"
                    variant="flat"
                    rounded="lg"
                    size="x-small"
                    :aria-expanded="openSubjectPlanNoteId === p.id"
                    @click.stop="
                      openSubjectPlanNoteId =
                        openSubjectPlanNoteId === p.id ? 0 : p.id
                    "
                    >备注 · {{ previewPlanNote(p.note, 24)
                    }}<VTooltip
                      activator="parent"
                      :model-value="openSubjectPlanNoteId === p.id"
                      :open-on-hover="false"
                      :open-on-focus="false"
                      :open-on-click="false"
                      location="top"
                      transition="scale-transition"
                      content-class="subject-tooltip today-note-tooltip"
                      ><p>{{ p.note }}</p></VTooltip
                    ></VBtn
                  >
                </div>
                <small
                  >时段 {{ p.duration }} 分钟 · 学习
                  {{ plannedStudyMinutes(p) }} 分钟<span v-if="p.breakMinutes">
                    · 休息 {{ p.breakMinutes }} 分钟</span
                  >
                  ·
                  {{
                    p.weekdays === "0,1,2,3,4,5,6"
                      ? "每天"
                      : p.weekdays === "1,2,3,4,5"
                        ? "周一至周五"
                        : p.weekdays
                            .split(",")
                            .map((x) => weekNames[Number(x)])
                            .join("、")
                  }}</small
                >
              </div>
            </div>
          </div>
        </div>
      </div></VCard
    ></VDialog
  >
</template>

<style scoped>
.subject-plans-dialog :deep(.plan-note-chip.v-btn) {
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
.subject-plans-dialog :deep(.plan-note-chip .v-btn__content) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subject-plans-dialog {
  background: var(--surface-card) !important;
  color: var(--ink) !important;
}
.subject-plans-list {
  max-height: min(68vh, 620px);
  overflow-y: auto;
  padding: 8px 18px 18px;
}
</style>
