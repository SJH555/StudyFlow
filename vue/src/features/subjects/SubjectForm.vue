<script setup lang="ts">
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { useSubjectDraft } from "./useSubjectDraft";
import { useSubjectsStore } from "../../stores/subjects";
import { colorPalette } from "../../shared/constants";
const {
  subjectEditId,
  subjectName,
  subjectColor,
  subjectSortEnabled,
  subjectSortInput,
  showSubjectColors,
  showSubject,
} = useSubjectDraft();
const subjectsStore = useSubjectsStore();
const { nextSubjectSortOrder } = storeToRefs(subjectsStore);
const { addSubject } = subjectsStore;
const subjectSortNumber = computed({
  get: () => Number(subjectSortInput.value) || null,
  set: (value: number | null) => {
    subjectSortInput.value = value == null ? "" : String(value);
  },
});
</script>

<template>
  <VDialog
    v-model="showSubject"
    max-width="480"
    :retain-focus="true"
    @keydown.enter.exact.prevent="addSubject"
  >
    <VCard
      class="subject-form-dialog"
      rounded="xl"
      @keydown.enter.exact.prevent="addSubject"
    >
      <VCardTitle class="subject-form-title">{{
        subjectEditId ? "编辑学习项目" : "添加学习项目"
      }}</VCardTitle>
      <VCardText class="subject-form-content">
        <VTextField
          v-model="subjectName"
          class="form-name-input"
          label=""
          aria-label="学习项目名称"
          placeholder="请输入项目名称"
          variant="solo-filled"
          density="comfortable"
          persistent-placeholder
          maxlength="50"
          autofocus
          hide-details="auto"
          @keydown.enter.prevent="addSubject"
        />
        <div class="subject-color-field">
          <div class="subject-form-label">项目颜色</div>
          <div class="subject-color-controls">
            <span
              class="subject-color-preview"
              :style="{ background: subjectColor }"
            ></span
            ><span class="subject-color-value">{{ subjectColor }}</span
            ><VMenu
              v-model="showSubjectColors"
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
                    @click="subjectColor = preset"
                    ><span
                      class="subject-palette-swatch"
                      :style="{ background: preset }"
                      ><VIcon
                        v-if="
                          subjectColor.toLowerCase() === preset.toLowerCase()
                        "
                        icon="mdi-check"
                        size="small"
                        color="white" /></span
                  ></VBtn>
                </div>
                <VDivider class="my-2" /><VColorPicker
                  v-model="subjectColor"
                  mode="hex"
                  hide-inputs
                  show-swatches
                  :swatches="[colorPalette]"
                  width="280" /></VCard
            ></VMenu>
          </div>
          <div class="subject-color-caption">新项目将自动使用预设颜色</div>
        </div>
        <div class="subject-sort-setting">
          <div>
            <strong>自定义排序</strong><small>设置序号的项目优先显示</small>
          </div>
          <VSwitch
            v-model="subjectSortEnabled"
            color="primary"
            inset
            hide-details
            aria-label="自定义排序"
            @update:model-value="
              subjectSortEnabled &&
              (subjectSortInput = String(nextSubjectSortOrder))
            "
          />
        </div>
        <div v-if="subjectSortEnabled" class="subject-sort-number-field">
          <span>排序号</span
          ><VNumberInput
            v-model="subjectSortNumber"
            aria-label="排序号"
            variant="outlined"
            density="comfortable"
            control-variant="stacked"
            :min="1"
            :step="1"
            hide-details="auto"
          />
        </div>
      </VCardText>
      <div class="subject-form-actions">
        <VBtn variant="text" rounded="lg" @click="showSubject = false"
          >取消</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          prepend-icon="mdi-content-save-outline"
          @click="addSubject"
          >保存</VBtn
        >
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.subject-form-dialog > .subject-form-actions {
  align-self: stretch;
}
.subject-form-dialog {
  background: var(--surface-card);
  color: var(--ink);
}
.subject-form-content {
  display: grid;
  gap: 18px;
  padding: 12px 24px 8px;
}
.subject-form-content :deep(.v-field) {
  background: var(--surface-card);
}
.subject-form-content {
  gap: 13px;
}
.subject-form-content > .v-input {
  margin: 0;
}
.subject-form-content > .v-input:has(input[type="number"]) :deep(.v-field) {
  border-radius: 10px;
  background: var(--surface-card);
}
.subject-form-content
  > .v-input:has(input[type="number"])
  :deep(.v-number-input__control button) {
  min-width: 38px;
  width: 38px;
  height: 38px;
}
.subject-form-content
  > .v-input:has(input[type="number"])
  :deep(.v-field__append-inner .v-btn) {
  min-width: 38px;
  width: 38px;
  height: 38px;
}
.subject-form-dialog {
  display: flex;
  flex-direction: column;
}
.subject-sort-number-field {
  display: grid;
  gap: 8px;
  color: var(--ink);
  font-size: 14px;
  font-weight: 600;
}
.subject-sort-number-field > .v-input {
  margin: 0;
}
.subject-sort-number-field :deep(.v-field) {
  border-radius: 10px;
  background: var(--surface-card);
}
</style>
