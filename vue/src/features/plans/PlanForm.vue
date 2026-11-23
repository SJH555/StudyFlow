<script setup lang="ts">
import { computed, useId } from "vue";
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlanDraft } from "./usePlanDraft";
import { usePlansForm } from "./usePlans";
import { usePlanTiming } from "./usePlanTiming";
import {
  colorPalette,
  weekNames,
  hourOptions,
  minuteOptions,
} from "../../shared/constants";
const titleId = useId();
const noticeId = useId();
const { page } = storeToRefs(useAppStore());
const { scheduledSubjects } = storeToRefs(useSubjectsStore());
const {
  planEditId,
  planSubject,
  planHour,
  planMinute,
  planDuration,
  planBreakEnabled,
  planBreakMinutes,
  planColor,
  planNote,
  repeatMode,
  selectedWeekdays,
  showPlanColors,
  showPlan,
  savingPlan,
} = usePlanDraft();
const { toggleWeekday, addPlan, planDisplayColor } = usePlansForm();
const planPickerColor = computed({
  get: () => planDisplayColor.value,
  set: (value: string) => {
    if (planColor.value || value.toLowerCase() !== planDisplayColor.value.toLowerCase()) {
      planColor.value = value;
    }
  },
});
const { startHour, startMinute, endHour, endMinute, timeError } = usePlanTiming(
  planHour,
  planMinute,
  planDuration,
);
const planDurationNumber = computed({
  get: () => Number(planDuration.value) || null,
  set: (value: number | null) => {
    planDuration.value = value == null ? "" : String(value);
  },
});
const planBreakMinutesNumber = computed({
  get: () => Number(planBreakMinutes.value) || null,
  set: (value: number | null) => {
    planBreakMinutes.value = value == null ? "" : String(value);
  },
});
</script>

<template>
  <VDialog v-model="showPlan" max-width="670" :persistent="savingPlan" :aria-labelledby="titleId" :aria-describedby="noticeId">
    <VCard class="plan-dialog-card" rounded="xl">
      <VCardTitle :id="titleId" class="plan-dialog-title">{{
        planEditId ? "编辑学习计划" : "安排学习时段"
      }}</VCardTitle>
      <p :id="noticeId" class="form-effective-notice">
        {{ planEditId ? "修改从明天起生效，今天的安排保留。" : "新计划从明天起生效。" }}
        今天可以在「今日计划」手动补充学习记录。
      </p>
      <VCardText class="plan-dialog-content">
        <VSelect
          v-model="planSubject"
          :items="scheduledSubjects.map((s) => ({ title: s.name, value: s.id }))"
          label="学习项目"
          placeholder="选择学习项目"
          variant="outlined"
          density="comfortable"
          hide-details="auto"
        />
        <VBtn
          v-if="!scheduledSubjects.length"
          variant="text"
          color="primary"
          @click="
            showPlan = false;
            page = 'subjects';
          "
          >明天没有可用项目，前往管理 ↗</VBtn
        >
        <div class="plan-dialog-time-row">
          <div class="plan-dialog-field">
            <span>开始时间</span>
            <div class="plan-time-select">
              <VSelect
                v-model="startHour"
                :items="hourOptions"
                aria-label="开始时间小时"
                variant="outlined"
                density="comfortable"
                hide-details
              /><span>:</span
              ><VSelect
                v-model="startMinute"
                :items="minuteOptions"
                aria-label="开始时间分钟"
                variant="outlined"
                density="comfortable"
                hide-details
              />
            </div>
          </div>
          <div class="plan-dialog-field">
            <span>结束时间</span>
            <div class="plan-time-select">
              <VSelect
                v-model="endHour"
                :items="hourOptions"
                aria-label="结束时间小时"
                variant="outlined"
                density="comfortable"
                hide-details
              /><span>:</span
              ><VSelect
                v-model="endMinute"
                :items="minuteOptions"
                aria-label="结束时间分钟"
                variant="outlined"
                density="comfortable"
                hide-details
              />
            </div>
          </div>
        </div>
        <div class="plan-dialog-field plan-duration-field">
          <span>计划时长（分钟）</span
          ><VNumberInput
            v-model="planDurationNumber"
            aria-label="计划时长（分钟）"
            variant="outlined"
            density="comfortable"
            control-variant="stacked"
            :min="1"
            :max="1440"
            :error-messages="timeError"
            hide-details="auto"
          />
        </div>
        <div class="plan-dialog-field">
          <span>重复方式</span
          ><VBtnToggle
            v-model="repeatMode"
            mandatory
            color="primary"
            variant="tonal"
            rounded="lg"
            class="plan-repeat-toggle"
            ><VBtn value="daily">每天</VBtn
            ><VBtn value="weekdays">周一至周五</VBtn
            ><VBtn value="custom">自选星期</VBtn></VBtnToggle
          >
          <div v-if="repeatMode === 'custom'" class="plan-weekday-buttons">
            <VBtn
              v-for="day in [1, 2, 3, 4, 5, 6, 0]"
              :key="day"
              size="small"
              rounded="lg"
              :variant="selectedWeekdays.includes(day) ? 'tonal' : 'text'"
              color="primary"
              @click="toggleWeekday(day)"
              >{{ weekNames[day] }}</VBtn
            >
          </div>
        </div>
        <div class="subject-sort-setting plan-dialog-break">
          <div>
            <strong>安排休息</strong><small>多次暂停共用这段休息时间</small>
          </div>
          <VSwitch
            v-model="planBreakEnabled"
            color="primary"
            inset
            hide-details
            aria-label="安排休息"
          />
        </div>
        <VNumberInput
          v-if="planBreakEnabled"
          v-model="planBreakMinutesNumber"
          label="休息时长（分钟）"
          variant="outlined"
          density="comfortable"
          control-variant="stacked"
          :min="1"
          :max="1439"
          hide-details="auto"
        />
        <div class="subject-color-field">
          <div class="subject-form-label">任务颜色</div>
          <div class="subject-color-controls">
            <span
              class="subject-color-preview"
              :style="{ background: planDisplayColor }"
            ></span
            ><span class="subject-color-value">{{ planColor }}</span
            ><VMenu
              v-model="showPlanColors"
              :close-on-content-click="false"
              location="bottom end"
              ><template #activator="{ props }"
                ><VBtn
                  v-bind="props"
                  variant="tonal"
                  color="primary"
                  rounded="lg"
                  prepend-icon="mdi-palette-outline"
                  >选择颜色</VBtn
                ></template
              ><VCard class="subject-color-menu" rounded="lg"
                ><div class="subject-palette">
                  <VBtn
                    v-for="(preset, index) in colorPalette"
                    :key="preset"
                    icon
                    variant="text"
                    rounded="lg"
                    :aria-label="`预设颜色 ${index + 1}`"
                    :title="preset"
                    @click="planColor = preset"
                    ><span
                      class="subject-palette-swatch"
                      :style="{ background: preset }"
                      ><VIcon
                        v-if="planColor.toLowerCase() === preset.toLowerCase()"
                        icon="mdi-check"
                        size="small"
                        color="white" /></span
                  ></VBtn>
                </div>
                <VDivider class="my-2" /><VColorPicker
                  v-model="planPickerColor"
                  mode="hex"
                  hide-inputs
                  show-swatches
                  :swatches="[colorPalette]"
                  width="280" /></VCard
            ></VMenu>
          </div>
          <div class="subject-color-caption">新计划将自动使用预设颜色</div>
        </div>

        <VTextarea
          v-model="planNote"
          label="备注（最多 100 字）"
          placeholder="例如：第 3 章导数定义与例题 1–5"
          variant="outlined"
          density="comfortable"
          rows="2"
          maxlength="100"
          hide-details="auto"
        /><small class="plan-note-count"
          >{{ Array.from(planNote).length }} / 100</small
        >
      </VCardText>
      <div class="subject-form-actions">
        <VBtn variant="text" rounded="lg" :disabled="savingPlan" @click="showPlan = false">取消</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          :disabled="savingPlan || !scheduledSubjects.length"
          :loading="savingPlan"
          @click="addPlan"
          >{{ planEditId ? "保存修改" : "保存计划" }}</VBtn
        >
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.plan-repeat-toggle {
  width: 100%;
  height: 44px;
}

.plan-repeat-toggle .v-btn {
  flex: 1;
  min-width: 0;
  text-transform: none;
  letter-spacing: 0;
}

.plan-weekday-buttons {
  display: grid;
  grid-template-columns: repeat(7,minmax(0,1fr));
  gap: 5px;
}

.plan-weekday-buttons .v-btn {
  min-width: 0;
  padding-inline: 2px;
  text-transform: none;
}

.plan-dialog-break {
  height: 60px;
  min-height: 60px;
  margin: 0;
}

.plan-dialog-time-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
  align-items: end;
}

.plan-dialog-time-row .plan-time-select {
  width: 100%;
  max-width: 240px;
}

.plan-dialog-time-row .plan-time-select .v-select {
  flex: 1;
  min-width: 0;
}

.plan-dialog-time-row .plan-time-select>span {
  flex: none;
}

.plan-duration-field {
  width: 100%;
  max-width: 240px;
}

.plan-duration-field>.v-input {
  margin: 0;
  min-width: 0;
}

@media (max-width:560px) {
  .plan-dialog-time-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width:560px) {
  .plan-dialog-time-row .plan-time-select {
    max-width: none;
  }
}
</style>
