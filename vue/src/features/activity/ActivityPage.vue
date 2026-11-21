<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { useSettingsStore } from "../../stores/settings";
import { useActivityStore } from "../../stores/activity";
import { exportHtml } from "../export/exportHtml";
import { hourOptions, minuteOptions } from "../../shared/constants";
import { time, formatDateChinese } from "../../shared/format";

const { selectedDate, todayKey } = storeToRefs(useAppStore());

const { settings } = storeToRefs(useSettingsStore());

const activityStore = useActivityStore();
const {
  activityDateMenu,
  activitySettingsSaving,
  activities,
  activityEnabled,
  activityStartHour,
  activityStartMinute,
  activitySkipRest,
  activityIntervalNumber,
  activitySettingsDirty,
} = storeToRefs(activityStore);
const {
  openActivity,
  saveActivitySchedule,
  toggleActivityReminder,
  pickActivityDate,
} = activityStore;
</script>

<template>
  <div class="page-heading">
    <div><h1>活动提醒</h1></div>
    <div class="heading-actions">
      <VMenu v-model="activityDateMenu" :close-on-content-click="false"
        ><template #activator="{ props }"
          ><VBtn
            v-bind="props"
            variant="outlined"
            color="primary"
            prepend-icon="mdi-calendar-month-outline"
            append-icon="mdi-chevron-down"
            class="activity-date-trigger"
            >{{ formatDateChinese(selectedDate) }}</VBtn
          ></template
        ><VDatePicker
          :key="todayKey"
          :model-value="selectedDate"
          color="primary"
          hide-title
          hide-header
          @update:model-value="pickActivityDate" /></VMenu
      ><VBtn
        class="activity-export-button"
        variant="outlined"
        color="primary"
        rounded="lg"
        prepend-icon="mdi-export-variant"
        @click="exportHtml('activity')"
        >导出 HTML</VBtn
      ><VBtn
        variant="tonal"
        color="primary"
        rounded="lg"
        prepend-icon="mdi-plus"
        @click="openActivity"
        >记录此刻</VBtn
      >
    </div>
  </div>
  <div class="activity-reminder-card">
    <div class="reminder-main">
      <span class="reminder-state">{{
        activityEnabled ? "提醒运行中" : "提醒已关闭"
      }}</span>
      <h2>定时记录当前活动</h2>
      <p>
        从 {{ settings.activityStartTime || "09:00" }} 开始，每隔
        {{ settings.activityInterval || 30 }} 分钟弹出记录框。
      </p>
      <VBtn
        class="reminder-main-button"
        :variant="activityEnabled ? 'tonal' : 'flat'"
        color="primary"
        rounded="lg"
        :prepend-icon="activityEnabled ? 'mdi-stop' : 'mdi-play'"
        @click="toggleActivityReminder"
        >{{ activityEnabled ? "关闭提醒" : "开始提醒" }}</VBtn
      >
    </div>
    <div class="reminder-fields">
      <label
        >开始时间
        <div class="plan-time-select">
          <VSelect
            v-model="activityStartHour"
            :items="hourOptions"
            aria-label="活动提醒开始小时"
            variant="outlined"
            color="primary"
            density="comfortable"
            :menu-props="{ contentClass: 'activity-time-menu', maxHeight: 280 }"
            hide-details
          /><span>:</span
          ><VSelect
            v-model="activityStartMinute"
            :items="minuteOptions"
            aria-label="活动提醒开始分钟"
            variant="outlined"
            color="primary"
            density="comfortable"
            :menu-props="{ contentClass: 'activity-time-menu', maxHeight: 280 }"
            hide-details
          /></div></label
      ><VNumberInput
        v-model="activityIntervalNumber"
        label="提醒间隔（分钟）"
        placeholder="例如 30"
        variant="outlined"
        density="comfortable"
        control-variant="stacked"
        :min="1"
        :max="1440"
        hide-details="auto"
      />
      <div class="activity-skip-rest-row">
        <div>
          <strong>跳过休息时间</strong
          ><small>计划内暂停休息、两项计划之间的空档不提醒。</small>
        </div>
        <VSwitch
          v-model="activitySkipRest"
          color="primary"
          inset
          hide-details
          aria-label="跳过休息时间"
        />
      </div>
      <VBtn
        class="reminder-save-button"
        variant="tonal"
        color="primary"
        rounded="lg"
        prepend-icon="mdi-content-save-outline"
        :loading="activitySettingsSaving"
        :disabled="!activitySettingsDirty || activitySettingsSaving"
        @click="saveActivitySchedule()"
        >保存设置</VBtn
      >
    </div>
  </div>
  <div class="activity-timeline">
    <div class="activity-day">
      {{ formatDateChinese(selectedDate) }} <span>·</span>
      {{ activities.length }} 次记录
    </div>
    <div class="activity-records-list">
      <div v-for="a in activities" :key="a.id" class="activity-row">
        <div class="activity-time">{{ time(a.happenedAt) }}</div>
        <div class="activity-line"><i></i></div>
        <div class="activity-entry">
          <div class="activity-entry-icon">✦</div>
          <div>
            <b>{{ a.content }}</b
            ><small
              >记录于
              {{
                new Date(a.happenedAt).toLocaleTimeString("zh-CN", {
                  hour: "2-digit",
                  minute: "2-digit",
                  second: "2-digit",
                })
              }}</small
            >
          </div>
        </div>
      </div>
      <div v-if="!activities.length" class="section-empty activity-empty">
        <div class="review-empty-illustration">
          <VIcon icon="mdi-bell-off-outline" />
        </div>
        <b>暂无活动记录</b>
      </div>
    </div>
  </div>
</template>

<style scoped>
.reminder-fields .plan-time-select > span {
  font-size: 18px;
}
.reminder-fields :deep(.plan-time-select .v-field input) {
  height: auto;
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

:global(.activity-time-menu.v-overlay__content) {
  border: 1px solid var(--outline-card);
  border-radius: 10px !important;
  background: var(--surface-card);
  box-shadow: 0 8px 24px color-mix(in srgb, var(--ink) 12%, transparent);
}
:global(.activity-time-menu .v-list) {
  padding: 6px;
  background: transparent;
}
:global(.activity-time-menu .v-list-item) {
  margin: 2px 0;
  border-radius: 8px;
}
:global(.activity-time-menu .v-list-item--active) {
  background: #f1f4fb !important;
  color: var(--ink) !important;
}
.activity-records-list {
  padding: 12px 18px;
  border: 1px dashed color-mix(in srgb, var(--ink) 24%, var(--outline-card));
  border-radius: 10px;
}
.activity-empty {
  margin: 0;
  min-height: 195px;
  align-content: center;
}
.activity-empty :deep(.v-icon) {
  color: var(--purple) !important;
}
.activity-empty b {
  color: var(--ink);
  font-size: 16px;
}
.activity-date-trigger {
  background: #fff;
  text-transform: none;
  letter-spacing: 0;
}
.reminder-fields label > input {
  background: #fff;
}
.reminder-fields :deep(.plan-time-select .v-field__overlay) {
  opacity: 0;
}
.reminder-main :deep(.reminder-main-button.v-btn) {
  text-transform: none;
  letter-spacing: 0;
}
.reminder-fields > .v-input {
  margin: 0;
}
.reminder-fields :deep(.v-field) {
  background: var(--surface-card);
  border-radius: 10px;
}
.reminder-fields :deep(.reminder-save-button.v-btn) {
  justify-self: end;
  min-width: 112px;
  height: 40px;
  text-transform: none;
  letter-spacing: 0;
}
.reminder-main :deep(.reminder-main-button.v-btn) {
  border-radius: 8px !important;
  background: var(--surface-hidden) !important;
  color: var(--purple) !important;
  box-shadow: none !important;
  font-weight: 600;
}
.reminder-main :deep(.reminder-main-button.v-btn .v-icon) {
  font-size: 24px;
}
.reminder-fields :deep(.v-number-input input) {
  height: auto;
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  color: inherit;
  font: inherit;
}
.reminder-fields :deep(.v-number-input .v-field input) {
  height: auto;
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  color: inherit;
  font: inherit;
}
.reminder-fields :deep(.v-number-input .v-field) {
  background: #fff;
  border: 1px solid var(--outline-card);
  border-radius: 10px;
  box-shadow: none;
}
.reminder-fields :deep(.plan-time-select .v-field) {
  background: var(--surface-app);
  border-radius: 10px;
  box-shadow: none;
}
.reminder-main :deep(.reminder-main-button.v-btn) {
  min-width: 176px;
  height: 50px;
  margin-top: 10px;
  padding-inline: 22px;
  font-size: 17px;
}
.heading-actions :deep(.activity-export-button.v-btn) {
  background: #f1f2f4 !important;
  color: #505866 !important;
  border: 1px solid #e0e3e9 !important;
  box-shadow: none !important;
  text-transform: none;
  letter-spacing: 0;
}
.reminder-fields :deep(.v-number-input .v-field__input) {
  padding-inline-start: 14px !important;
  padding-inline-end: 10px !important;
}
.activity-export-button.v-btn {
  background: #f1f2f4 !important;
  color: #505866 !important;
  border: 1px solid #e0e3e9 !important;
  box-shadow: none !important;
  text-transform: none;
  letter-spacing: 0;
}
.reminder-fields .plan-time-select {
  gap: 12px;
  margin-top: 11px;
}
.reminder-fields > .v-input {
  margin-top: 4px;
}
.reminder-fields :deep(.plan-time-select .v-field--focused) {
  background: var(--surface-app) !important;
}
.reminder-fields :deep(.plan-time-select .v-field--active) {
  background: var(--surface-app) !important;
}
.reminder-fields :deep(.plan-time-select .v-field--focused .v-field__outline) {
  --v-field-border-opacity: 1;
}
.reminder-fields :deep(.plan-time-select .v-field) {
  border: 0 !important;
}
.reminder-fields :deep(.plan-time-select .v-field--focused) {
  box-shadow: none !important;
  border: 0 !important;
}
.reminder-fields :deep(.plan-time-select .v-field__outline) {
  color: color-mix(in srgb, var(--ink) 50%, var(--surface-app));
  --v-field-border-opacity: 1;
}
.reminder-fields :deep(.plan-time-select .v-field--focused .v-field__outline) {
  color: var(--ink) !important;
}

.activity-timeline {
  border: 1px solid #eeede7;
  border-radius: 11px;
  padding: 20px 22px;
  background: var(--surface-app);
}
.activity-day {
  font-weight: 600;
  margin-bottom: 12px;
}
.activity-day span {
  color: #c5c2b9;
  margin: 0 4px;
}
.activity-row {
  display: grid;
}
.activity-time {
  color: #8c8a81;
}
.activity-line {
  position: relative;
  display: flex;
  justify-content: center;
}
.activity-line:before {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: #eae8e2;
}
.activity-line i {
  position: relative;
  margin-top: 16px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #9689e7;
  outline: 3px solid #eef1ff;
  z-index: 1;
}
.activity-entry {
  display: flex;
  align-items: center;
}
.activity-entry-icon {
  border-radius: 8px;
  display: grid;
  place-items: center;
}
.activity-entry b {
  font-weight: 500;
}
.activity-empty {
  margin-top: 10px;
}
.activity-row {
  grid-template-columns: 56px 17px 1fr;
  min-height: 78px;
}
.activity-time {
  font-size: 12px;
  padding-top: 15px;
}
.activity-entry {
  gap: 12px;
  padding: 9px 13px;
}
.activity-entry-icon {
  width: 34px;
  height: 34px;
}
.activity-entry b {
  font-size: 14px;
}
.activity-entry small {
  display: block;
  color: #aaa89f;
  font-size: 11px;
  margin-top: 4px;
}
.activity-day {
  font-size: 15px !important;
}
.activity-reminder-card {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(280px, 0.85fr);
  gap: 24px;
  margin-bottom: 22px;
  padding: 26px 30px;
  border: 1px solid #dce5ff;
  border-radius: 16px;
  background: var(--surface-app);
}
.reminder-main {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}
.reminder-state {
  font-size: 12px;
  font-weight: 650;
}
.reminder-main h2 {
  font-size: 23px;
  margin: 10px 0 5px;
}
.reminder-main p {
  font-size: 14px;
  margin: 0 0 20px;
}
.reminder-main-button {
  min-width: 160px;
  height: 47px;
  border: 0;
  border-radius: 10px;
  background: #2932e1;
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  box-shadow: 0 6px 16px #2932e135;
  cursor: pointer;
}
.reminder-fields {
  display: grid;
  align-content: center;
  padding: 16px 18px;
  border: 1px solid #e8edfa;
  border-radius: 12px;
  background: var(--surface-app);
  gap: var(--space-5);
}
.reminder-fields label {
  display: block;
  font-size: 13px;
  color: #5b6474;
  font-weight: 600;
}
.reminder-fields select {
  height: 39px;
  border: 1px solid #dce3ef;
  border-radius: 8px;
  padding: 0 10px;
  background: var(--surface-card);
  color: #3d4656;
  font: inherit;
}
.reminder-fields select {
  min-width: 0;
  flex: 1;
}
.reminder-fields input {
  height: 39px;
  border: 1px solid #dce3ef;
  border-radius: 8px;
  padding: 0 10px;
  background: var(--surface-card);
  color: #3d4656;
  font: inherit;
  display: block;
  width: 100%;
  margin-top: 7px;
}
@media (max-width: 1100px) {
  .activity-reminder-card {
    grid-template-columns: 1fr;
  }
}
.activity-reminder-card {
  border-color: #dce5ff;
  box-shadow: 0 8px 25px #243b8b0a;
}
.reminder-state {
  color: #5368ce;
}
.reminder-main p {
  color: #68758c;
}
.activity-entry-icon {
  background: #eef1ff;
  color: var(--activity-accent);
}
.activity-skip-rest-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 11px 0;
  border-top: 1px solid var(--outline-card);
}
.activity-skip-rest-row > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.activity-skip-rest-row strong {
  font-size: 13px;
  color: var(--ink);
}
.activity-skip-rest-row small {
  font-size: 12px;
  line-height: 1.45;
  color: var(--muted);
}
.activity-skip-rest-row :deep(.v-switch) {
  flex: none;
}

.reminder-fields {
  border-color: color-mix(in srgb, var(--ink) 22%, var(--outline-card));
  box-shadow: 0 2px 8px color-mix(in srgb, var(--ink) 3%, transparent);
}
</style>
