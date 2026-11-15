import { computed, ref } from "vue";
import { defineStore } from "pinia";
import {
  createGoal,
  acceptGoal,
  deleteGoal,
  listGoals,
  setGoalCompleted,
  updateGoal,
} from "../features/goals/api";
import type { Goal, GoalInput } from "../features/goals/types";
import { dateKey } from "../shared/format";
import { useAppStore } from "./app";

// 阶段目标：负责目标加载、编辑、完成状态与按截止日期排序。
export const useGoalsStore = defineStore("goals", () => {
  const appStore = useAppStore();
  const goals = ref<Goal[]>([]);

  const activeGoals = computed(() =>
    goals.value
      .filter((goal) => !goal.accepted)
      .sort((a, b) => a.deadline.localeCompare(b.deadline) || a.id - b.id),
  );
  const acceptedGoals = computed(() =>
    goals.value
      .filter((goal) => goal.accepted)
      .sort((a, b) => a.deadline.localeCompare(b.deadline) || a.id - b.id),
  );
  const dueSoonCount = computed(
    () =>
      activeGoals.value.filter((goal) => {
        const remaining = daysRemaining(goal.deadline);
        return remaining >= 0 && remaining <= 7;
      }).length,
  );

  function daysRemaining(deadline: string) {
    const [year, month, day] = deadline.split("-").map(Number);
    const target = Date.UTC(year, month - 1, day);
    const todayKey = dateKey(new Date(appStore.now));
    const [todayYear, todayMonth, todayDay] = todayKey.split("-").map(Number);
    const today = Date.UTC(todayYear, todayMonth - 1, todayDay);
    return Math.round((target - today) / 86_400_000);
  }

  function createdDate(goal: Goal) {
    return dateKey(new Date(goal.createdAt));
  }

  // 截止日和制定日均按本地自然日计，恰好剩余一半时进入提醒状态。
  function countdownTone(goal: Goal) {
    const remaining = daysRemaining(goal.deadline);
    if (remaining < 0) return "overdue";
    const [year, month, day] = goal.deadline.split("-").map(Number);
    const [createdYear, createdMonth, createdDay] = createdDate(goal).split("-").map(Number);
    const total = Math.round(
      (Date.UTC(year, month - 1, day) - Date.UTC(createdYear, createdMonth - 1, createdDay)) /
        86_400_000,
    );
    return remaining * 2 <= total ? "soon" : "upcoming";
  }

  async function fetch() {
    goals.value = await listGoals();
  }

  async function save(input: GoalInput) {
    if (input.id) await updateGoal({ ...input, id: input.id });
    else await createGoal(input);
    await fetch();
  }

  async function setCompleted(id: number, completed: boolean) {
    await setGoalCompleted(id, completed);
    await fetch();
  }

  async function accept(id: number, completed: boolean) {
    await acceptGoal(id, completed);
    await fetch();
  }

  async function remove(id: number) {
    await deleteGoal(id);
    await fetch();
  }

  return {
    goals,
    activeGoals,
    acceptedGoals,
    dueSoonCount,
    daysRemaining,
    createdDate,
    countdownTone,
    fetch,
    save,
    setCompleted,
    accept,
    remove,
  };
});
