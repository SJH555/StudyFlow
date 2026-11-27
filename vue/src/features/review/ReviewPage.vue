<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { useReviewStore } from "../../stores/review";
import { fmt, formatDateChinese } from "../../shared/format";
import { hourOptions, minuteOptions } from "../../shared/constants";
import GoalsPanel from "../goals/GoalsPanel.vue";
import SectionSubnav from "../../shared/SectionSubnav.vue";
const {
  summaryTab,
  reviewHistory,
  reviewMode,
  reviewThreshold,
  reviewCheckHour,
  reviewCheckMinute,
  reviewEnabledDraft,
  reviewPageHeading,
  reviewSettingsSaving,
  reviewSettingsDirty,
} = storeToRefs(useReviewStore());
const { saveReviewSettings } = useReviewStore();

// 检查时间以“小时 + 分钟”存储，VSelect 使用字符串选项，这里做 number ↔ string 转换。
const reviewCheckHourModel = computed({
  get: () => String(reviewCheckHour.value ?? 6).padStart(2, "0"),
  set: (value: string) => {
    reviewCheckHour.value = Number(value);
  },
});
const reviewCheckMinuteModel = computed({
  get: () => String(reviewCheckMinute.value ?? 0).padStart(2, "0"),
  set: (value: string) => {
    reviewCheckMinute.value = Number(value);
  },
});

const navigationItems = [
  { key: "goals", title: "阶段目标", icon: "mdi-flag-checkered" },
  {
    key: "learning",
    title: "进度追因",
    icon: "mdi-clipboard-text-clock-outline",
  },
] as const;

function selectSection(key: string) {
  if (key === "goals" || key === "learning") summaryTab.value = key;
}
</script>

<template>
  <VApp class="settings-app review-app section-app">
    <div class="section-page-header">
      <div class="section-page-icon">
        <VIcon icon="mdi-chart-timeline-variant" />
      </div>
      <div>
        <span class="section-eyebrow">LEARNING PROGRESS</span>
        <h1>{{ reviewPageHeading.title }}</h1>
        <p>{{ reviewPageHeading.description }}</p>
      </div>
    </div>
    <div class="section-layout joined-panels">
      <SectionSubnav
        label="学情追踪"
        label-icon="mdi-chart-timeline-variant"
        navigation-label="学情追踪分类"
        :active-key="summaryTab"
        :items="navigationItems"
        @select="selectSection"
      />
      <div v-if="summaryTab === 'goals'" class="section-column">
        <GoalsPanel />
      </div>
      <div v-else class="section-column">
        <VCard
          class="section-card review-unified-panel"
          variant="outlined"
          rounded="lg"
          elevation="0"
        >
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>启用自动检查</strong
              ><small>开启后从当天开始计算；关闭期间不计算。</small>
            </div>
            <VSwitch
              v-model="reviewEnabledDraft"
              color="primary"
              inset
              hide-details
              aria-label="启用进度追因自动检查"
            />
          </div>
          <VDivider class="review-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>检查时间</strong
              ><small>每天到达该时间后开始检查前一天的学习完成情况。</small>
            </div>
            <div class="plan-time-select review-time-select">
              <VSelect
                v-model="reviewCheckHourModel"
                :items="hourOptions"
                aria-label="检查时间小时"
                variant="outlined"
                density="comfortable"
                hide-details
              /><span>:</span
              ><VSelect
                v-model="reviewCheckMinuteModel"
                :items="minuteOptions"
                aria-label="检查时间分钟"
                variant="outlined"
                density="comfortable"
                hide-details
              />
            </div>
          </div>
          <VDivider class="review-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>检查范围</strong
              ><small>汇总全天时长，或分别检查每个学习项目。</small>
            </div>
            <VBtnToggle
              v-model="reviewMode"
              mandatory
              color="primary"
              variant="tonal"
              density="compact"
              aria-label="检查范围"
              ><VBtn value="total" prepend-icon="mdi-chart-donut">总时长</VBtn
              ><VBtn value="project" prepend-icon="mdi-view-grid-outline"
                >分项目</VBtn
              ></VBtnToggle
            >
          </div>
          <VDivider class="review-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>最低完成比例</strong
              ><small>实际时长达到计划时长的设定比例即视为达标。</small>
            </div>
            <VNumberInput
              v-model="reviewThreshold"
              class="review-number-input"
              :min="1"
              :max="100"
              :step="1"
              :precision="0"
              control-variant="stacked"
              density="compact"
              variant="solo-filled"
              flat
              hide-details
              suffix="%"
              aria-label="最低完成比例"
            />
          </div>
          <div class="section-card-actions">
            <VBtn
              class="review-save-button"
              color="primary"
              variant="tonal"
              size="small"
              prepend-icon="mdi-content-save-outline"
              :disabled="!reviewSettingsDirty"
              :loading="reviewSettingsSaving"
              @click="saveReviewSettings"
              >保存设置</VBtn
            >
          </div>
          <VDivider class="review-section-divider" />
          <section
            class="review-history-section"
            aria-labelledby="review-history-title"
          >
            <div class="section-card-header">
              <div class="section-card-title">
                <VIcon icon="mdi-history" />
                <div>
                  <h2 id="review-history-title">每日检查结果</h2>
                  <p>按日期查看达标结果和已填写的追因。</p>
                </div>
              </div>
              <VChip size="small" label variant="tonal" color="primary"
                >共 {{ reviewHistory.length }} 条记录</VChip
              >
            </div>
            <div class="review-records-list">
              <div v-if="!reviewHistory.length" class="section-empty">
                <div class="review-empty-illustration">
                  <VIcon icon="mdi-clipboard-text-clock-outline" />
                </div>
                <b>暂无检查结果</b>
              </div>
              <article
                v-for="entry in reviewHistory"
                :key="entry.day"
                class="review-record"
              >
                <div class="review-record-top">
                  <b>{{ formatDateChinese(entry.day) }} · {{ entry.status === "passed" ? "已达标" : "未达标" }}</b
                  ><small
                    >计划 {{ fmt(entry.plannedSeconds) }} · 实际
                    {{ fmt(entry.actualSeconds) }} · 目标
                    {{ entry.threshold }}%</small
                  >
                </div>
                <div
                  v-for="item in entry.items"
                  :key="item.subjectId"
                  class="review-record-reason"
                  :class="{ 'review-record-reason-by-project': entry.mode === 'project' }"
                >
                  <div class="review-record-project">
                    <strong>{{ entry.mode === "total" ? "未达标原因" : item.subjectName }}</strong>
                    <small v-if="entry.mode === 'project'">
                      计划 {{ fmt(item.plannedSeconds) }} · 实际 {{ fmt(item.actualSeconds) }}
                    </small>
                  </div>
                  <span>{{ item.reason }}</span>
                </div>
              </article>
            </div>
          </section>
        </VCard>
      </div>
    </div>

  </VApp>
</template>

<style scoped>
.section-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.section-card-title {
  display: flex;
  align-items: flex-start;
  min-width: 0;
}
.section-card-title > .v-icon {
  flex: none;
  color: var(--section-primary);
}
.section-card-title h2 {
  margin: 0;
  color: var(--ink);
  line-height: 1.35;
}
.section-card-title p {
  margin: 4px 0 0;
  line-height: 1.5;
}
.section-card-header > .v-chip {
  flex: none;
}
.section-card-header > .v-btn {
  flex: none;
}
.section-card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 15px;
  padding: 14px 22px;
}
.section-card-actions > span {
  color: var(--section-muted);
  font-size: 12px;
}
.section-card-actions :deep(.v-btn) {
  min-width: 130px;
}

.review-time-select {
  inline-size: 240px;
  margin-block-start: 0;
  flex: none;
  gap: var(--space-2);
}

.review-time-select :deep(.v-select) {
  flex: 1;
  min-inline-size: 0;
}

.review-time-select :deep(.v-field) {
  min-block-size: var(--size-field);
  border: 0;
  border-radius: var(--shape-medium);
  background: var(--review-surface);
  box-shadow: none;
}

.review-time-select :deep(.v-field__outline) {
  color: var(--outline-card);
  --v-field-border-opacity: 0.72;
}

.review-time-select :deep(.v-field--focused .v-field__outline) {
  color: var(--review-primary);
  --v-field-border-opacity: 1;
}

.review-time-select :deep(.v-field__overlay) {
  opacity: 0;
}

.review-time-select :deep(.v-field__input) {
  min-inline-size: 0;
  flex-wrap: nowrap;
  padding-inline: var(--space-2);
  font-variant-numeric: tabular-nums;
}

.review-time-select :deep(.v-select__selection) {
  flex: none;
  max-inline-size: none;
}

.review-time-select :deep(.v-select__selection-text) {
  overflow: visible;
  text-overflow: clip;
  white-space: nowrap;
  color: var(--ink);
  font-weight: var(--weight-medium);
}

.review-unified-panel .review-time-select :deep(.v-field) {
  min-block-size: 40px;
  border-radius: var(--shape-small);
}

/* 页面元素使用 scoped，仅穿透 Vuetify 内部结构。 */

.review-app.v-application {
  display: block;
  min-height: 0;
  background: transparent !important;
  font-family: inherit;
}

.review-app :deep(.v-application__wrap) {
  min-height: 0;
}

:where(.review-app) .v-btn {
  text-transform: none;
  letter-spacing: 0;
  font-weight: 600;
}

:where(.review-app) .v-btn:focus-visible {
  outline: 2px solid var(--review-accent);
  outline-offset: 2px;
}

.review-number-input {
  flex: none;
}

.review-record {
  border-bottom: 1px solid var(--review-outline);
}

.review-record:last-child {
  border-bottom: 0;
}

@media (max-width: 1050px) {
  .review-record-top {
    flex-wrap: wrap;
  }
}

.review-number-input {
  width: 142px;
}

.review-records-list {
  max-height: 380px;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0 22px;
  scrollbar-color: var(--section-outline) transparent;
  scrollbar-width: thin;
}

.review-record {
  padding: 16px 0;
}

.review-record + .review-record {
  border-top: 1px solid var(--section-outline);
}

.review-record-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
}

.review-record-top b {
  font-size: 14px;
}

.review-record-top small {
  color: var(--section-muted);
  font-size: 12px;
}

.review-record-reason {
  display: grid;
  grid-template-columns: 110px minmax(0, 1fr);
  gap: 12px;
  margin-top: 10px;
  font-size: 13px;
  line-height: 1.5;
}

.review-record-reason-by-project {
  grid-template-columns: minmax(0, 180px) minmax(0, 1fr);
}

.review-record-project {
  display: grid;
  gap: 2px;
  overflow-wrap: anywhere;
}

.review-record-project small {
  color: var(--section-muted);
  font-size: 12px;
}

.review-record-reason strong {
  color: var(--section-primary);
}

.review-record-reason span {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 960px) {
  .review-record-top {
    flex-wrap: wrap;
  }
}

.review-app.v-application {
  --review-surface: #ffffff;
  --review-outline: var(--review-line);
  --review-ink: #202b40;
  --review-muted: #687891;
  --review-accent: var(--review-primary);
  color: var(--review-ink);
}

:where(.review-app) .section-page-header {
  gap: 27px;
  margin: 0 0 29px;
}

:where(.review-app) .section-page-icon {
  width: 82px;
  height: 82px;
  border-radius: 25px;
  background: linear-gradient(145deg, #e8efff, #e5ecff);
  box-shadow: inset 0 1px 0 #ffffffb8;
  color: var(--review-primary);
}

:where(.review-app) .section-page-icon .v-icon {
  font-size: 37px;
}

:where(.review-app) .section-eyebrow {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 7px;
  background: #e7edff;
  color: var(--review-primary);
  font-size: 12px;
  letter-spacing: 0.02em;
}

:where(.review-app) .section-page-header h1 {
  margin: 6px 0 0;
  color: var(--review-ink);
  font-size: 32px;
  line-height: 1.15;
}

:where(.review-app) .section-page-header p {
  margin-top: 5px;
  color: var(--review-ink);
  font-size: 14px;
}

:where(.review-app) .section-layout {
  max-width: none;
}

:where(.review-app) .section-card.v-card {
  background: var(--review-surface);
  border: 1px solid var(--review-line);
  border-radius: 16px !important;
  box-shadow: 0 8px 26px #344b8a0b;
}

:where(.review-app) .section-card .v-divider {
  border-color: #edf1f9;
}

:where(.review-app) .section-card-header {
  min-height: 103px;
  padding: 23px 23px;
}

:where(.review-app) .section-card-title {
  gap: 14px;
}

:where(.review-app) .section-card-title > .v-icon {
  font-size: 25px;
  color: var(--review-primary);
}

:where(.review-app) .section-card-title h2 {
  font-size: 20px;
  color: var(--review-ink);
}

:where(.review-app) .section-card-title p {
  margin-top: 6px;
  color: var(--review-muted);
  font-size: 13px;
}

:where(.review-app) .section-card-header .v-chip {
  font-size: 12px;
  font-weight: 700;
}

:where(.review-app) .section-setting-row {
  min-height: 91px;
  padding: 15px 23px;
}

:where(.review-app) .section-setting-copy strong {
  font-size: 16px;
  color: var(--review-ink);
}

:where(.review-app) .section-setting-copy small {
  font-size: 13px;
  color: var(--review-muted);
}

:where(.review-app) .section-setting-row .v-btn-toggle {
  border: 1px solid var(--review-line);
  border-radius: 10px;
  overflow: hidden;
}

:where(.review-app) .section-setting-row .v-btn-toggle .v-btn {
  min-width: 112px;
  height: 44px;
}

:where(.review-app) .review-number-input {
  width: 162px;
}

:where(.review-app) .review-number-input :deep(.v-field) {
  min-height: 48px;
  border: 1px solid var(--review-line);
  border-radius: 10px;
  background: #f8faff;
  box-shadow: inset 0 1px 3px #243c7a0d;
}

:where(.review-app) .review-number-input :deep(.v-field__input) {
  font-size: 17px;
  font-weight: 650;
  color: var(--review-ink);
  font-variant-numeric: tabular-nums;
}

:where(.review-app) .review-number-input :deep(.v-field__suffix) {
  color: var(--review-muted);
  font-size: 15px;
}

:where(.review-app) .review-number-input .v-btn {
  color: var(--review-ink);
}

:where(.review-app) .section-card-actions {
  min-height: 74px;
  padding: 12px 22px 20px;
}

:where(.review-app) .section-card-actions .v-btn {
  min-width: 146px;
  min-height: 44px;
  border-radius: 8px;
}

:where(.review-app) .section-empty {
  min-height: 195px;
  align-content: center;
  gap: 6px;
  padding: 20px;
}

:where(.review-app) .section-empty b {
  font-size: 16px;
  color: var(--review-ink);
}

:where(.review-app) .section-empty span {
  font-size: 13px;
  color: var(--review-muted);
}

:where(.review-app) .review-records-list {
  max-height: 390px;
  min-height: 195px;
  padding: 0 23px;
}

:where(.review-app) .review-record-reason strong {
  color: var(--review-primary);
}

:where(.review-app) .section-layout {
  grid-template-columns: 166px minmax(0, 1fr);
  gap: 12px;
}

:where(.review-app) .review-unified-panel.v-card {
  background: #ffffffed;
  border: 1px solid var(--review-line);
  border-radius: 10px !important;
  box-shadow: none;
}

:where(.review-app) .review-unified-panel .v-divider {
  border-color: #e8edf7;
}

:where(.review-app) .review-unified-panel .section-card-header {
  min-height: 78px;
  padding: 15px 16px;
}

:where(.review-app) .review-unified-panel .section-card-title h2 {
  font-size: 18px;
}

:where(.review-app) .review-unified-panel .section-setting-row {
  padding: 11px 16px;
}

:where(.review-app) .review-unified-panel .section-card-actions {
  padding: 9px 16px;
}

:where(.review-app) .review-unified-panel .section-card-actions .v-btn {
  border-radius: 7px;
  font-size: 12px;
}

:where(.review-app)
  .review-unified-panel
  .section-card-actions
  .v-btn
  :deep(.v-icon) {
  font-size: 16px;
}

:where(.review-app) .review-unified-panel .review-number-input {
  width: 130px;
}

:where(.review-app) .review-unified-panel .review-number-input :deep(.v-field) {
  min-height: 38px;
  border-radius: 8px;
}

:where(.review-app)
  .review-unified-panel
  .review-number-input
  :deep(.v-field__input) {
  min-height: 38px;
  padding-top: 4px;
  padding-bottom: 4px;
  font-size: 14px;
}

:where(.review-app)
  .review-unified-panel
  .review-number-input
  :deep(.v-field__suffix) {
  font-size: 12px;
}

:where(.review-app) .review-unified-panel .review-records-list {
  padding: 0 16px;
}

@media (max-width: 950px) {
  :where(.review-app) .section-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}

:where(.review-app) .review-unified-panel {
  min-width: 0;
  background: transparent;
}

:where(.review-app) .review-unified-panel .section-card-actions .v-btn {
  width: 105px;
  min-width: 105px;
  height: 39px;
  min-height: 39px;
  padding-inline: 8px;
}

:where(.review-app) .review-row-divider.v-divider {
  border-color: #f0f3fa;
  opacity: 0.8;
}

:where(.review-app) .review-unified-panel .section-setting-row {
  min-height: 92px;
  padding-top: 16px;
  padding-bottom: 16px;
}

:where(.review-app) .review-section-divider.v-divider {
  opacity: 1;
  border-color: var(--review-line);
  border-top-width: 1px;
}

:where(.review-app) .section-page-header {
  padding-left: 20px;
}

:where(.review-app) .review-unified-panel .section-card-actions {
  min-height: 59px;
  padding-top: 9px;
  padding-bottom: 11px;
}

.review-app.settings-app.v-application {
  --section-primary: var(--purple);
  --section-muted: #637087;
  color: var(--ink);
}

.review-app.settings-app .section-page-header {
  gap: 15px;
  margin: 0;
  padding: var(--space-3) 32px;
  border: 1px solid var(--outline-card);
  border-bottom: 0;
  border-radius: 8px;
  background: rgb(247 249 254) !important;
}

.review-app.settings-app .section-page-header h1 {
  margin: 2px 0 0;
  color: var(--ink);
  font-size: 30px;
  line-height: 1.25;
}

.review-app.settings-app .section-page-header p {
  margin-top: 3px;
  color: var(--section-muted);
  font-size: 14px;
}

.review-app.settings-app .section-eyebrow {
  display: block;
  padding: 0;
  border-radius: 0;
  background: transparent;
  color: var(--section-primary);
  font-size: 10px;
  letter-spacing: 0.11em;
}

.review-app.settings-app .section-layout {
  grid-template-columns:
    var(--section-nav-width, clamp(190px, 26%, 240px))
    minmax(0, 1fr);
  align-items: stretch;
  gap: 0;
  max-width: none;
  min-height: 0;
  border: 0;
  border-radius: 8px 8px 8px 0;
  background: var(--canvas-study);
  box-shadow: -3px 0 6px color-mix(in srgb, var(--ink) 9%, transparent);
}

.review-app.settings-app .section-column {
  display: grid;
  min-width: 0;
}

.review-app.settings-app .review-unified-panel.section-card.v-card {
  height: 100%;
  min-height: 0;
  padding: 0 26px 18px;
  border: 1px solid var(--outline-card);
  border-top: 0;
  border-bottom: 0;
  border-left: 0;
  border-radius: var(--joined-panel-seam-radius) 8px 8px
    var(--joined-panel-seam-radius) !important;
  background: #fff;
  box-shadow: none;
}

.review-app.settings-app .review-unified-panel .section-card-header {
  min-height: 0;
  padding: 26px 0 20px;
}

.review-app.settings-app .review-unified-panel .section-setting-row {
  min-height: 76px;
  padding: 12px 0;
}

.review-app.settings-app .review-unified-panel .section-card-actions {
  padding: 12px 0 20px;
}

.review-app.settings-app .review-unified-panel .review-records-list {
  padding: 0;
}

.review-app.settings-app .review-history-section .review-records-list {
  margin: 14px 0 4px;
  padding: 12px 18px;
  border: 1px dashed color-mix(in srgb, var(--ink) 24%, var(--outline-card));
  border-radius: 10px;
}

.review-app.settings-app .review-unified-panel .v-divider {
  border-color: var(--outline-card);
}

.review-app.settings-app
  :deep(.review-unified-panel .section-card-header + .v-divider) {
  opacity: 1;
}

@media (max-width: 1100px) {
  .review-app.settings-app .section-layout {
    grid-template-columns: var(--section-nav-width, 190px) minmax(0, 1fr);
  }
}

@media (max-width: 1100px) {
  .review-app.settings-app .review-unified-panel.section-card.v-card {
    padding-inline: 18px;
  }
}

@media (max-width: 950px) {
  .review-app.settings-app .section-layout {
    grid-template-columns: minmax(0, 1fr);
    overflow-y: auto;
  }
}

@media (max-width: 950px) {
  .review-app.settings-app .review-unified-panel.section-card.v-card {
    border-left: 1px solid var(--outline-card);
    border-radius: var(--joined-panel-seam-radius)
      var(--joined-panel-seam-radius) 8px 8px !important;
  }
}

.review-app.settings-app .section-column {
  grid-template-rows: minmax(0, 1fr);
  min-height: 0;
  overflow: hidden;
}

.review-app.settings-app .review-unified-panel.section-card.v-card {
  max-height: 100%;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}

.review-app.settings-app .review-records-list {
  max-height: none;
  min-height: 0;
  overflow: visible;
}

.review-app.settings-app .review-unified-panel.section-card.v-card {
  padding-top: 12px;
}

:where(.section-setting-row) .v-number-input {
  flex: none;
}
@media (max-width: 960px) {
  :where(.section-setting-row) .v-number-input {
    margin-left: auto;
  }
}
.section-card-header {
  padding: 26px 0 20px;
}
.section-card-title {
  gap: 15px;
}
.section-card-title > .v-icon {
  margin-top: 2px;
  font-size: 26px;
}
.section-card-title h2 {
  font-size: 22px;
  font-weight: 750;
}
.section-card-title p {
  margin-top: 7px;
  font-size: 14px;
  margin-left: -41px;
  color: var(--section-muted);
}
</style>
