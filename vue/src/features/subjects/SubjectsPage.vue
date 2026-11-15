<script setup lang="ts">
import ListActionButton from "../../shared/ListActionButton.vue";
import { storeToRefs } from "pinia";
import { useSubjectsStore } from "../../stores/subjects";
import { usePlansStore } from "../../stores/plans";
import { useSubjectDraft } from "./useSubjectDraft";
const { subjects, allSubjects } = storeToRefs(useSubjectsStore());
const { plans } = storeToRefs(usePlansStore());
const {
  subjectTab,
  subjectPlanTarget,
  showSubjectPlans,
  openSubjectPlanNoteId,
  deleteSubjectTarget,
} = useSubjectDraft();
const { beginAddSubject, beginEditSubject, archiveSubject, restoreSubject } =
  useSubjectsStore();
</script>

<template>
  <div class="page-heading">
    <div><h1>学习项目</h1></div>
    <VBtn
      color="primary"
      variant="tonal"
      rounded="lg"
      prepend-icon="mdi-plus"
      @click="beginAddSubject"
      >添加项目</VBtn
    >
  </div>
  <section class="panel subject-directory">
    <div class="subject-directory-tabs">
      <VBtnToggle
        v-model="subjectTab"
        mandatory
        color="primary"
        variant="tonal"
        density="comfortable"
        aria-label="学习项目状态"
        ><VBtn value="enabled"
          >启用
          <span class="subject-tab-count">{{ subjects.length }}</span></VBtn
        ><VBtn value="hidden"
          >隐藏
          <span class="subject-tab-count">{{
            allSubjects.filter((s) => s.archived && !s.deleted).length
          }}</span></VBtn
        ></VBtnToggle
      >
    </div>
    <div class="subject-directory-head">
      <span>项目名称</span><span>关联计划</span><span>操作</span>
    </div>
    <div
      v-for="s in subjectTab === 'enabled'
        ? subjects
        : allSubjects.filter((x) => x.archived && !x.deleted)"
      :key="s.id"
      class="subject-directory-row"
    >
      <div class="subject-directory-name">
        <span class="subject-dot" :style="{ background: s.color }"></span
        ><b>{{ s.name }}</b>
      </div>
      <div class="subject-directory-plans">
        <VBtn
          v-if="plans.some((p) => p.subjectId === s.id)"
          class="subject-plan-trigger"
          variant="tonal"
          color="primary"
          size="small"
          rounded="lg"
          prepend-icon="mdi-calendar-clock-outline"
          append-icon="mdi-chevron-right"
          @click="
            subjectPlanTarget = s;
            openSubjectPlanNoteId = 0;
            showSubjectPlans = true;
          "
          >{{ plans.filter((p) => p.subjectId === s.id).length }} 个计划</VBtn
        ><span v-else class="subject-no-plans" aria-label="无关联计划">—</span>
      </div>
      <div class="subject-directory-actions">
        <ListActionButton
          action="edit"
          v-if="subjectTab === 'enabled'"
          @click="beginEditSubject(s)"
        /><span v-else class="subject-action-spacer" aria-hidden="true"></span
        ><VBtn
          :prepend-icon="
            subjectTab === 'enabled' ? 'mdi-eye-off-outline' : 'mdi-eye-outline'
          "
          variant="tonal"
          color="primary"
          size="small"
          rounded="lg"
          @click="
            subjectTab === 'enabled' ? archiveSubject(s) : restoreSubject(s)
          "
          >{{ subjectTab === "enabled" ? "隐藏" : "启用" }}</VBtn
        ><span class="subject-delete-wrap"
          ><ListActionButton
            action="delete"
            :disabled="s.hasData"
            @click="deleteSubjectTarget = s"
          /><VTooltip
            v-if="s.hasData"
            activator="parent"
            location="top"
            content-class="subject-tooltip"
            >存在关联数据，无法删除</VTooltip
          ></span
        >
      </div>
    </div>
    <div
      v-if="subjectTab === 'enabled' && !subjects.length"
      class="subtle-empty"
    >
      没有启用的学习项目
    </div>
    <div
      v-if="
        subjectTab === 'hidden' &&
        !allSubjects.some((s) => s.archived && !s.deleted)
      "
      class="subtle-empty"
    >
      没有隐藏的学习项目
    </div>
  </section>
</template>

<style scoped>
.subject-directory-name .subject-dot {
  flex: none;
}

.subject-directory {
  width: 100%;
  padding: 22px 24px;
  background: #fff;
}
.subject-directory-tabs {
  margin-bottom: 20px;
}
.subject-directory-tabs :deep(.v-btn-toggle) {
  border: 1px solid var(--outline-card);
  border-radius: 9px;
  overflow: hidden;
}
.subject-directory-tabs :deep(.v-btn) {
  min-width: 110px;
  text-transform: none;
}
.subject-tab-count {
  display: inline-grid;
  place-items: center;
  min-width: 21px;
  height: 21px;
  margin-left: 8px;
  padding: 0 5px;
  border-radius: 11px;
  background: var(--surface-card);
  font-size: 12px;
}
.subject-directory-head {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.subject-directory-row {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.subject-directory-head span:first-child {
  max-width: calc(50% - 105px);
}
.subject-directory-head {
  min-height: 38px;
  padding: 0 18px;
  color: var(--section-muted);
  font-size: 12px;
  font-weight: 650;
}
.subject-directory-head span:nth-child(2) {
  position: absolute;
  left: calc(50% - 35px);
  width: 160px;
  transform: translateX(-50%);
  justify-content: center;
  text-align: center;
}
.subject-directory-row .subject-directory-plans {
  position: absolute;
  left: calc(50% - 35px);
  width: 160px;
  transform: translateX(-50%);
  justify-content: center;
  text-align: center;
}
.subject-directory-head span:last-child {
  width: 240px;
  margin-left: auto;
  text-align: center;
}
.subject-directory-row {
  min-height: 70px;
  padding: 8px 18px;
  border-top: 1px solid var(--outline-card);
}
.subject-directory-name {
  max-width: calc(50% - 105px);
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
}
.subject-directory-plans {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
}
.subject-directory-actions {
  align-items: center;
  min-width: 0;
}
.subject-directory-name b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.subject-directory-plans {
  justify-content: center;
  color: var(--section-muted);
}
.subject-directory-plans .subject-plan-trigger {
  min-width: 132px;
  justify-content: space-between;
  text-transform: none;
}
.subject-no-plans {
  display: inline-block;
  min-width: 132px;
  color: var(--section-muted);
  text-align: center;
  font-size: 18px;
  line-height: 1;
}
.subject-directory-actions {
  display: grid;
  grid-template-columns: repeat(3, 76px);
  justify-content: end;
  gap: 6px;
  margin-left: auto;
}
.subject-directory-actions :deep(.v-btn) {
  text-transform: none;
}
.subject-directory-actions > .v-btn {
  width: 76px;
  height: 36px;
  padding-inline: 6px;
}
.subject-delete-wrap :deep(.v-btn) {
  width: 76px;
  height: 36px;
  padding-inline: 6px;
}
.subject-directory-actions :deep(.v-btn__prepend) {
  margin-inline-end: 4px;
}
.subject-action-spacer {
  width: 76px;
  height: 36px;
}
.subject-delete-wrap {
  width: 76px;
  height: 36px;
  display: inline-flex;
}
@media (max-width: 1050px) {
  .subject-directory-head {
    gap: 8px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-row {
    gap: 8px;
  }
}
@media (max-width: 1050px) {
  .subject-directory {
    padding: 16px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-head span:nth-child(2) {
    left: calc(50% - 24px);
    width: 132px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-row .subject-directory-plans {
    left: calc(50% - 24px);
    width: 132px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-head span:last-child {
    width: 164px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-actions {
    grid-template-columns: repeat(3, 52px);
    gap: 4px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-actions > .v-btn {
    width: 52px;
  }
}
@media (max-width: 1050px) {
  .subject-delete-wrap :deep(.v-btn) {
    width: 52px;
  }
}
@media (max-width: 1050px) {
  .subject-action-spacer {
    width: 52px;
  }
}
@media (max-width: 1050px) {
  .subject-delete-wrap {
    width: 52px;
  }
}
@media (max-width: 1050px) {
  .subject-directory-actions :deep(.v-btn__prepend) {
    display: none;
  }
}
</style>
