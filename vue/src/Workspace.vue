<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useAppStore } from "./stores/app";
import { useSettingsStore } from "./stores/settings";
import { useSubjectsStore } from "./stores/subjects";
import { useSessionsStore } from "./stores/sessions";
import { useActivityStore } from "./stores/activity";
import { useReviewStore } from "./stores/review";
import { useSubjectDraft } from "./features/subjects/useSubjectDraft";
import { usePlanDraft } from "./features/plans/usePlanDraft";
import { usePlansForm } from "./features/plans/usePlans";
import { useManualDraft } from "./features/sessions/useManualDraft";
import { useWindowFrame } from "./features/window/useWindowFrame";
import { useSidebar } from "./features/navigation/useSidebar";
import { useKeyboardShortcuts } from "./features/interactions/useKeyboardShortcuts";
import { useBootstrap } from "./features/useBootstrap";
import { useLifecycle } from "./features/useLifecycle";
import TodayPage from "./features/today/TodayPage.vue";
import PlansPage from "./features/plans/PlansPage.vue";
import SubjectsPage from "./features/subjects/SubjectsPage.vue";
import StatsPage from "./features/stats/StatsPage.vue";
import ActivityPage from "./features/activity/ActivityPage.vue";
import ReviewPage from "./features/review/ReviewPage.vue";
import SettingsPage from "./features/settings/SettingsPage.vue";
// 应用级挂载的弹窗与浮层（实现归各业务模块）。
import SubjectForm from "./features/subjects/SubjectForm.vue";
import SubjectPlansDialog from "./features/subjects/SubjectPlansDialog.vue";
import SubjectDeleteDialog from "./features/subjects/SubjectDeleteDialog.vue";
import PlanForm from "./features/plans/PlanForm.vue";
import PlanDeleteDialog from "./features/plans/PlanDeleteDialog.vue";
import ReminderDialogs from "./features/plans/ReminderDialogs.vue";
import ManualForm from "./features/sessions/ManualForm.vue";
import StudyConfirmDialogs from "./features/sessions/StudyConfirmDialogs.vue";
import ActivityDialogs from "./features/activity/ActivityDialogs.vue";
import SettingsDialogs from "./features/settings/SettingsDialogs.vue";
import ReviewDialog from "./features/review/ReviewDialog.vue";
import RestOrb from "./features/interactions/RestOrb.vue";
import AppNotices from "./features/interactions/AppNotices.vue";

// 组合根：各领域能力与全局订阅在根部单次装配。
const appStore = useAppStore();
const { page, loading, focusMode } = storeToRefs(appStore);

const settingsStore = useSettingsStore();
const subjectsStore = useSubjectsStore();
const sessionsStore = useSessionsStore();
const activityStore = useActivityStore();
const reviewStore = useReviewStore();
const subjectDraft = useSubjectDraft();
const planDraft = usePlanDraft();
const manualDraft = useManualDraft();
const plansForm = usePlansForm();

const { windowsTitlebar, windowsMaximised, windowAction } = useWindowFrame(
  appStore.setError,
);
const {
  primaryNavigation,
  sidebarMinWidth,
  sidebarMaxWidth,
  sidebarWidth,
  sidebarCompact,
  beginSidebarResize,
  moveSidebarResize,
  endSidebarResize,
  resizeSidebarByKey,
} = useSidebar();

const reviewRefs = storeToRefs(reviewStore);
const sessionsRefs = storeToRefs(sessionsStore);
const activityRefs = storeToRefs(activityStore);
const settingsRefs = storeToRefs(settingsStore);

const { handleDialogKeydown } = useKeyboardShortcuts({
  reviewPending: reviewRefs.reviewPending,
  reviewBulkChoice: reviewRefs.reviewBulkChoice,
  submitBulkReview: reviewStore.submitBulkReview,
  submitReview: reviewStore.submitReview,
  openPlanNoteId: planDraft.openPlanNoteId,
  reminderActive: activityRefs.reminderActive,
  addActivity: activityStore.addActivity,
  deletePlanTarget: planDraft.deletePlanTarget,
  resetTarget: settingsRefs.resetTarget,
  deleteSubjectTarget: subjectDraft.deleteSubjectTarget,
  deleteSessionTarget: sessionsRefs.deleteSessionTarget,
  showFinishConfirm: sessionsRefs.showFinishConfirm,
  pendingStartPlan: sessionsRefs.pendingStartPlan,
  showTaskReminder: sessionsRefs.showTaskReminder,
  showBreakReminder: sessionsRefs.showBreakReminder,
  showBreakExhausted: sessionsRefs.showBreakExhausted,
  showActivity: activityRefs.showActivity,
  showSubject: subjectDraft.showSubject,
  addSubject: subjectsStore.addSubject,
  showPlan: planDraft.showPlan,
  addPlan: plansForm.addPlan,
  showManual: manualDraft.showManual,
  addManual: sessionsStore.addManual,
  setError: appStore.setError,
});

const { load } = useBootstrap();
useLifecycle({ load, handleDialogKeydown });
</script>

<template>
  <div
    class="shell"
    :class="{
      'windows-frame': windowsTitlebar,
      'windows-maximised': windowsTitlebar && windowsMaximised,
      'sidebar-compact': sidebarCompact,
    }"
    :style="{
      '--sidebar-width': sidebarWidth + 'px',
      '--sidebar-progress':
        (sidebarWidth - sidebarMinWidth) / (sidebarMaxWidth - sidebarMinWidth),
    }"
  >
    <div v-if="windowsTitlebar && !focusMode" class="windows-titlebar">
      <div class="windows-titlebar-drag" @dblclick="windowAction('maximise')">
        <img src="/app-icon.png" alt="" /><span>StudyFlow</span>
      </div>
      <div class="windows-titlebar-actions">
        <button
          type="button"
          aria-label="最小化"
          title="最小化"
          @click="windowAction('minimise')"
        >
          −</button
        ><button
          type="button"
          aria-label="最大化或还原"
          title="最大化或还原"
          @click="windowAction('maximise')"
        >
          □</button
        ><button
          type="button"
          class="windows-titlebar-close"
          aria-label="关闭窗口"
          title="关闭窗口"
          @click="windowAction('close')"
        >
          ×
        </button>
      </div>
    </div>
    <aside class="sidebar app-sidebar">
      <div class="brand">
        <img class="brand-logo" src="/app-icon.png" alt="" />
        <div><b>StudyFlow</b></div>
      </div>
      <VList
        class="app-primary-nav"
        density="comfortable"
        nav
        aria-label="主导航"
      >
        <VListItem
          v-for="item in primaryNavigation"
          :key="item.key"
          :title="item.label"
          :aria-label="item.label"
          :prepend-icon="item.icon"
          :active="page === item.key"
          color="primary"
          rounded="lg"
          @click="page = item.key"
          ><VTooltip v-if="sidebarCompact" activator="parent" location="end">{{
            item.label
          }}</VTooltip></VListItem
        >
      </VList>
      <div class="sidebar-bottom">
        <VDivider /><VList
          class="app-primary-nav"
          density="comfortable"
          nav
          aria-label="设置导航"
          ><VListItem
            title="设置"
            aria-label="设置"
            prepend-icon="mdi-cog-outline"
            :active="page === 'settings'"
            color="primary"
            rounded="lg"
            @click="page = 'settings'"
            ><VTooltip v-if="sidebarCompact" activator="parent" location="end"
              >设置</VTooltip
            ></VListItem
          ></VList
        >
      </div>
      <div
        class="nav-resize-handle sidebar-resize-handle"
        role="separator"
        aria-label="调整侧边栏宽度"
        aria-orientation="vertical"
        :aria-valuenow="sidebarWidth"
        :aria-valuemin="sidebarMinWidth"
        :aria-valuemax="sidebarMaxWidth"
        tabindex="0"
        @pointerdown.stop="beginSidebarResize"
        @pointermove.stop="moveSidebarResize"
        @pointerup.stop="endSidebarResize"
        @pointercancel.stop="endSidebarResize"
        @click.stop.prevent
        @keydown="resizeSidebarByKey"
      />
    </aside>
    <main
      class="main"
      :class="{
        'today-main': page === 'today',
        'subjects-main': page === 'subjects',
        'plans-main': page === 'plans',
        'stats-main': page === 'stats',
        'activity-main': page === 'activity',
        'review-main': page === 'summary',
        'settings-main': page === 'settings' || page === 'summary',
      }"
    >
      <div
        class="content"
        :class="{
          'today-content': page === 'today',
          'review-content': page === 'summary',
          'settings-content': page === 'settings',
        }"
        v-if="!loading"
      >
        <TodayPage v-if="page === 'today'" />
        <PlansPage v-else-if="page === 'plans'" /><SubjectsPage
          v-else-if="page === 'subjects'"
        /><StatsPage v-else-if="page === 'stats'" />
        <ActivityPage v-else-if="page === 'activity'" />
        <ReviewPage v-else-if="page === 'summary'" />
        <SettingsPage v-else />
      </div>
      <div v-else class="loading">
        <div class="spinner"></div>
        <span>正在打开你的学习空间…</span>
      </div>
    </main>
  </div>
  <SubjectPlansDialog />
  <SubjectForm /><PlanForm /><ManualForm />
  <ActivityDialogs />
  <SettingsDialogs /><PlanDeleteDialog /><StudyConfirmDialogs /><ReminderDialogs />
  <SubjectDeleteDialog />
  <ReviewDialog />
  <RestOrb />
  <AppNotices />
</template>

<style scoped>
.content.review-content {
  position: relative;
  max-width: none;
  min-height: 0;
  padding: 8px 0;
  overflow: visible;
}
.content.settings-content {
  max-width: none;
  padding: 8px 0 8px;
}
.content.settings-content {
  background: #fff;
}
.content.settings-content {
  background: transparent;
}

.review-main {
  --review-primary: #2863f4;
  --review-ink: #202b40;
  --review-muted: #687891;
  --review-line: #dce4f6;
  --review-surface: #ffffff;
  --review-soft: #e8eeff;
  background:
    radial-gradient(
      ellipse 66% 38% at 96% 0%,
      #dfe7ff 0%,
      #edf2ff 56%,
      transparent 100%
    ),
    linear-gradient(125deg, #f9fbff 0%, #f4f7ff 100%);
}
</style>
