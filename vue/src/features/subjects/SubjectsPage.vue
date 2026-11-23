<script setup lang="ts">
import DirectoryStatusButton from "../../shared/DirectoryStatusButton.vue";
import DirectorySwitch from "../../shared/DirectorySwitch.vue";
import RecordBadge from "../../shared/RecordBadge.vue";
import ListActionButton from "../../shared/ListActionButton.vue";
import { computed } from "vue";
import { storeToRefs } from "pinia";
import { sameSubjectContent, useSubjectsStore } from "../../stores/subjects";
import { usePlansStore } from "../../stores/plans";
import { useSubjectDraft } from "./useSubjectDraft";
const { allSubjects, managedSubjects } = storeToRefs(useSubjectsStore());
const directorySubjects = computed(() => {
  const plannedById = new Map(managedSubjects.value.map((subject) => [subject.id, subject]));
  return allSubjects.value.flatMap((today) => {
    const planned = plannedById.get(today.id);
    if (!planned || today.deleted) return [];
    return [{
      ...(planned.deleted ? today : planned),
      archivedToday: today.archived,
      pendingDeletion: planned.deleted,
      pending: planned.archived !== today.archived,
      pendingEdit: !sameSubjectContent(today, planned),
    }];
  });
});
const enabledSubjects = computed(() => directorySubjects.value.filter((subject) => !subject.archivedToday));
const stoppedSubjects = computed(() => directorySubjects.value.filter((subject) => subject.archivedToday));
const { managedPlansBySubject } = storeToRefs(usePlansStore());
const {
  subjectTab,
  subjectPlanTarget,
  showSubjectPlans,
  openSubjectPlanNoteId,
} = useSubjectDraft();
const { beginAddSubject, beginEditSubject, archiveSubject, restoreSubject, beginDeleteSubject } =
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
    <DirectorySwitch
      v-model="subjectTab"
      label="学习项目状态"
      :options="[
        { value: 'enabled', label: '启用', count: enabledSubjects.length },
        { value: 'hidden', label: '停用', count: stoppedSubjects.length },
      ]"
    />
    <div class="subject-directory-head">
      <span>项目名称</span><span>关联计划</span><span>操作</span>
    </div>
    <div
      v-for="s in subjectTab === 'enabled' ? enabledSubjects : stoppedSubjects"
      :key="s.id"
      class="subject-directory-row"
    >
      <div class="subject-directory-name">
        <span class="subject-dot" :style="{ background: s.color }"></span>
        <b>{{ s.name }}</b>
        <RecordBadge v-if="s.pendingDeletion" icon="mdi-clock-outline">明日起删除</RecordBadge>
        <RecordBadge v-else-if="s.pending" icon="mdi-clock-outline">
          {{ s.archived ? "明日起停用" : "明日生效" }}
        </RecordBadge>
        <RecordBadge v-else-if="s.pendingEdit" icon="mdi-clock-outline">明日起变更</RecordBadge>
      </div>
      <div class="subject-directory-plans">
        <VBtn
          v-if="managedPlansBySubject.get(s.id)?.length"
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
          >{{ managedPlansBySubject.get(s.id)?.length }} 个计划</VBtn
        ><span v-else class="subject-no-plans" aria-label="无关联计划">—</span>
      </div>
      <div v-if="!s.pendingDeletion" class="subject-directory-actions">
        <ListActionButton
          action="edit"
          v-if="subjectTab === 'enabled'"
          @click="beginEditSubject(s)"
        /><span v-else class="subject-action-spacer" aria-hidden="true"></span
        ><DirectoryStatusButton
          :enabled="!s.archivedToday"
          :undo="s.pending"
          :aria-label="`${s.pending ? '撤销' : s.archivedToday ? '启用' : '停用'} ${s.name}`"
          @click="s.archived ? restoreSubject(s) : archiveSubject(s)"
        /><ListActionButton action="delete" @click="beginDeleteSubject(s)" />
      </div>
    </div>
    <div
      v-if="subjectTab === 'enabled' && !enabledSubjects.length"
      class="subtle-empty"
    >
      没有启用的学习项目
    </div>
    <div
      v-if="subjectTab === 'hidden' && !stoppedSubjects.length"
      class="subtle-empty"
    >
      没有停用的学习项目
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
