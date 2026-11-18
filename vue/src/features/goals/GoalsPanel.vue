<script setup lang="ts">
import { computed, ref } from "vue";
import { storeToRefs } from "pinia";
import type { Goal } from "./types";
import { formatDateChinese, formatRemainingDays } from "../../shared/format";
import { useAppStore } from "../../stores/app";
import { useGoalsStore } from "../../stores/goals";
import ListActionButton from "../../shared/ListActionButton.vue";
import SummaryCard from "../../shared/SummaryCard.vue";
import GoalDialog from "./GoalDialog.vue";

const appStore = useAppStore();
const goalsStore = useGoalsStore();
const { activeGoals, acceptedGoals, dueSoonCount } = storeToRefs(goalsStore);
const showAccepted = ref(false);
const sortOrder = ref<"deadline" | "created">("deadline");
const editorOpen = ref(false);
const selectedGoal = ref<Goal | null>(null);
const deleteTarget = ref<Goal | null>(null);
const acceptTarget = ref<Goal | null>(null);
const acceptDialogOpen = computed({
  get: () => acceptTarget.value !== null,
  set: (open: boolean) => {
    if (!open) acceptTarget.value = null;
  },
});
const deleteDialogOpen = computed({
  get: () => deleteTarget.value !== null,
  set: (open: boolean) => {
    if (!open) deleteTarget.value = null;
  },
});
const visibleGoals = computed(() => {
  const goals = showAccepted.value ? acceptedGoals.value : activeGoals.value;
  if (sortOrder.value === "deadline") return goals;
  return [...goals].sort(
    (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt) || a.id - b.id,
  );
});

function openEditor(goal: Goal | null = null) {
  selectedGoal.value = goal;
  editorOpen.value = true;
}

function countdownColor(goal: Goal) {
  const tone = goalsStore.countdownTone(goal);
  return tone === "overdue" ? "error" : tone === "soon" ? "warning" : "var(--goal-upcoming-color)";
}

function goalStateClass(goal: Goal) {
  if (goal.accepted)
    return goal.completed ? "goal-state-completed" : "goal-state-not-completed";
  const tone = goalsStore.countdownTone(goal);
  if (tone === "overdue") return "goal-state-overdue";
  if (tone === "soon") return "goal-state-soon";
  return "goal-state-upcoming";
}

async function setCompleted(goal: Goal, completed: boolean) {
  try {
    await goalsStore.setCompleted(goal.id, completed);
  } catch (error) {
    appStore.setError(error);
  }
}

async function acceptResult(completed: boolean) {
  if (!acceptTarget.value) return;
  try {
    await goalsStore.accept(acceptTarget.value.id, completed);
    acceptTarget.value = null;
  } catch (error) {
    appStore.setError(error);
  }
}

async function removeGoal() {
  if (!deleteTarget.value) return;
  try {
    await goalsStore.remove(deleteTarget.value.id);
    deleteTarget.value = null;
  } catch (error) {
    appStore.setError(error);
  }
}
</script>

<template>
  <VCard
    tag="section"
    variant="flat"
    class="goals-page"
    aria-labelledby="goals-title"
  >
    <div class="goals-page-heading">
      <h2 id="goals-title">阶段目标</h2>
      <VBtn
        color="primary"
        variant="tonal"
        prepend-icon="mdi-plus"
        rounded="lg"
        @click="openEditor()"
      >
        新建目标
      </VBtn>
    </div>

    <div class="goals-summary" aria-label="目标概况">
      <SummaryCard label="进行中的目标" icon="mdi-check" tone="primary">{{
        activeGoals.length
      }}</SummaryCard>
      <SummaryCard label="7 天内截止" icon="mdi-clock-outline" tone="warning">{{
        dueSoonCount
      }}</SummaryCard>
      <SummaryCard label="已验收" icon="mdi-check" tone="success">{{
        acceptedGoals.length
      }}</SummaryCard>
    </div>

    <VCard
      tag="section"
      variant="flat"
      class="goals-current-panel"
      aria-labelledby="current-goals-title"
    >
      <div class="goals-list-heading">
        <h3 id="current-goals-title">
          {{ showAccepted ? "已验收目标" : "当前目标" }}
        </h3>
        <div class="goals-list-controls">
          <VMenu
            location="bottom start"
            :width="180"
            :min-width="180"
            :max-width="180"
          >
            <template #activator="{ props }">
              <VBtn
                v-bind="props"
                class="goals-sort-button"
                variant="tonal"
                color="primary"
                rounded="lg"
                size="small"
                prepend-icon="mdi-sort"
                append-icon="mdi-chevron-down"
                :aria-label="`目标排序：${sortOrder === 'deadline' ? '截止日期近到远' : '创建日期新到旧'}`"
              >
                {{ sortOrder === "deadline" ? "截止优先" : "新建优先" }}
              </VBtn>
            </template>
            <VList
              class="goals-sort-menu"
              density="compact"
              rounded="lg"
              aria-label="目标排序方式"
            >
              <VListItem
                title="截止日期：近到远"
                :active="sortOrder === 'deadline'"
                color="primary"
                rounded="lg"
                @click="sortOrder = 'deadline'"
              />
              <VListItem
                title="创建日期：新到旧"
                :active="sortOrder === 'created'"
                color="primary"
                rounded="lg"
                @click="sortOrder = 'created'"
              />
            </VList>
          </VMenu>
          <VBtnToggle
            v-model="showAccepted"
            mandatory
            color="primary"
            variant="tonal"
            density="compact"
            aria-label="目标状态"
          >
            <VBtn :value="false">进行中</VBtn>
            <VBtn :value="true">已验收</VBtn>
          </VBtnToggle>
        </div>
      </div>

      <small v-if="!showAccepted" class="goals-home-hint"
        >首页只展示截止日期最近的目标</small
      >

      <div v-if="visibleGoals.length" class="goals-list">
        <VCard
          tag="article"
          variant="flat"
          v-for="goal in visibleGoals"
          :key="goal.id"
          class="goal-card"
          :class="goalStateClass(goal)"
        >
          <div class="goal-card-copy">
            <h4>{{ goal.title }}</h4>
            <p v-if="goal.note">{{ goal.note }}</p>
            <span class="goal-deadline"
              >截止日期　{{ formatDateChinese(goal.deadline) }}</span
            >
            <span class="goal-deadline"
              >制定日期　{{
                formatDateChinese(goalsStore.createdDate(goal))
              }}</span
            >
          </div>
          <div class="goal-card-actions">
            <VChip
              size="small"
              label
              variant="tonal"
              :class="goalStateClass(goal)"
              :color="
                goal.accepted
                  ? goal.completed
                    ? 'success'
                    : 'error'
                  : countdownColor(goal)
              "
            >
              {{
                goal.accepted
                  ? goal.completed
                    ? "已完成"
                    : "未完成"
                  : formatRemainingDays(goalsStore.daysRemaining(goal.deadline))
              }}
            </VChip>
            <ListActionButton
              v-if="!goal.accepted"
              action="accept"
              @click="acceptTarget = goal"
            />
            <ListActionButton action="edit" @click="openEditor(goal)" />
            <ListActionButton
              v-if="goal.accepted"
              action="reopen"
              @click="setCompleted(goal, false)"
            />
            <ListActionButton action="delete" @click="deleteTarget = goal" />
          </div>
        </VCard>
      </div>
      <div v-else class="goals-empty">
        <VIcon
          :icon="showAccepted ? 'mdi-check-circle-outline' : 'mdi-flag-outline'"
        />
        <strong>{{
          showAccepted ? "还没有已验收的目标" : "还没有阶段目标"
        }}</strong>
        <span>{{
          showAccepted
            ? "验收目标后会收录在这里。"
            : "添加一个目标，给近期学习设定清晰方向。"
        }}</span>
        <VBtn
          v-if="!showAccepted"
          variant="tonal"
          color="primary"
          @click="openEditor()"
        >
          新建目标
        </VBtn>
      </div>
    </VCard>
  </VCard>

  <GoalDialog v-model="editorOpen" :goal="selectedGoal" />

  <VDialog v-model="acceptDialogOpen" max-width="420">
    <VCard class="plan-confirm-dialog plan-dialog-card" rounded="xl">
      <VCardTitle class="plan-dialog-title">该目标是否完成？</VCardTitle>
      <VCardText class="plan-confirm-content"
        ><p>“{{ acceptTarget?.title }}”</p></VCardText
      >
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn
          variant="tonal"
          color="error"
          rounded="lg"
          @click="acceptResult(false)"
          >否</VBtn
        >
        <VBtn
          variant="tonal"
          color="success"
          rounded="lg"
          @click="acceptResult(true)"
          >是</VBtn
        >
      </div>
    </VCard>
  </VDialog>

  <VDialog v-model="deleteDialogOpen" max-width="420">
    <VCard class="plan-confirm-dialog plan-dialog-card" rounded="xl">
      <VCardTitle class="plan-dialog-title">删除这个目标？</VCardTitle>
      <VCardText class="plan-confirm-content">
        <p>“{{ deleteTarget?.title }}”将从目标列表中移除。</p>
      </VCardText>
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn variant="text" rounded="lg" @click="deleteDialogOpen = false"
          >取消</VBtn
        >
        <VBtn variant="tonal" color="error" rounded="lg" @click="removeGoal">
          删除目标
        </VBtn>
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.goals-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  min-inline-size: 0;
  min-block-size: 0;
  max-block-size: 100%;
  padding: var(--space-6);
  overflow: clip;
  container: goals-page / inline-size;
  border: 1px solid var(--outline-card);
  border-radius: var(--shape-large);
  background: var(--surface-app);
  box-shadow: var(--elevation-panel);
}

.goals-page > * {
  flex: none;
}

.goals-page-heading,
.goals-list-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--space-3);
}

.goals-page-heading h2,
.goals-list-heading h3 {
  margin: 0;
  color: var(--ink);
  font-weight: var(--weight-strong);
}

.goals-page-heading h2 {
  font-size: var(--text-heading);
}

.goals-list-heading h3 {
  font-size: var(--text-title);
}

.goals-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);
}

.goals-current-panel.v-card,
.goal-card.v-card {
  border: 1px solid var(--outline-card);
  border-radius: var(--shape-medium);
  background: var(--surface-app);
  box-shadow: var(--elevation-item);
}

.goals-current-panel.v-card {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  gap: var(--space-4);
  min-block-size: 0;
  padding: var(--space-5);
  overflow: clip;
}

.goals-list-heading .v-btn-toggle {
  border: 1px solid var(--outline-card);
  border-radius: var(--shape-small);
}

.goals-list-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
  max-inline-size: 100%;
}

.goals-sort-button.v-btn {
  inline-size: 180px;
  max-inline-size: 100%;
  justify-content: space-between;
  min-width: 0;
  text-transform: none;
  letter-spacing: 0;
}

:global(.goals-sort-menu.v-list) {
  inline-size: 100%;
  max-inline-size: 100%;
}

:global(.goals-sort-menu .v-list-item-title) {
  text-align: left;
  font-size: var(--text-body);
  white-space: normal;
  overflow-wrap: anywhere;
}

.goals-home-hint {
  color: var(--muted);
  font-size: var(--text-small);
}

.goals-current-panel > .goals-list-heading {
  flex: none;
}

.goals-list-heading .v-btn {
  padding-inline: var(--space-3);
  font-size: var(--text-small);
}

.goals-list {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  gap: var(--space-3);
  min-block-size: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}

.goal-card.v-card {
  --goal-accent: var(--goal-upcoming-color);
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  min-inline-size: 0;
  border-inline-start: var(--space-1) solid var(--goal-accent);
}

.goal-card.goal-state-soon {
  --goal-accent: rgb(var(--v-theme-warning));
}

.goal-card.goal-state-overdue {
  --goal-accent: rgb(var(--v-theme-error));
}

.goal-card.goal-state-completed {
  --goal-accent: rgb(var(--v-theme-success));
}

.goal-card.goal-state-not-completed {
  --goal-accent: rgb(var(--v-theme-error));
}

.goal-card-copy {
  display: grid;
  gap: var(--space-2);
  flex: 1 1 200px;
  min-inline-size: 0;
  overflow-wrap: anywhere;
}

.goal-card-copy h4 {
  margin: 0;
  color: var(--ink);
  font-size: var(--text-title);
  font-weight: var(--weight-strong);
}

.goal-card-copy p {
  margin: 0;
  color: var(--muted);
  font-size: var(--text-small);
  line-height: 1.45;
}

.goal-deadline {
  color: var(--muted);
  font-size: var(--text-caption);
}

.goal-card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: var(--space-2);
  max-inline-size: 100%;
}

.goal-card-actions .v-chip {
  border-radius: var(--shape-small);
  font-weight: var(--weight-medium);
}

.goals-empty {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: var(--space-2);
  min-block-size: 220px;
  padding: var(--space-5);
  text-align: center;
  color: var(--muted);
  font-size: var(--text-small);
}

.goals-empty > .v-icon {
  color: var(--review-primary);
  font-size: var(--size-list-action);
}

.goals-empty strong {
  color: var(--ink);
  font-size: var(--text-title);
}

@container goals-page (inline-size < 400px) {
  .goals-summary {
    grid-template-columns: 1fr;
  }

  .goal-card-actions {
    justify-content: flex-start;
  }
}

.review-app.settings-app .section-column > .goals-page.v-card {
  height: 100%;
  border-top: 0;
  border-bottom: 0;
  border-left: 0;
  border-radius: var(--joined-panel-seam-radius) var(--joined-panel-radius)
    var(--joined-panel-radius) var(--joined-panel-seam-radius);
  box-shadow: none;
}

@media (max-width: 950px) {
  .review-app.settings-app .section-column > .goals-page.v-card {
    border-left: 1px solid var(--outline-card);
    border-radius: var(--joined-panel-seam-radius)
      var(--joined-panel-seam-radius) var(--joined-panel-radius)
      var(--joined-panel-radius);
  }
}
</style>
