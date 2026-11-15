import { request } from "../../api";
import type { Goal, GoalInput } from "./types";

export const listGoals = () => request<Goal[]>("goals");
export const createGoal = (input: GoalInput) =>
  request<Goal>("goals", "POST", input);
export const updateGoal = (input: GoalInput) =>
  request<Goal>(`goals/${input.id}`, "PUT", input);
export const setGoalCompleted = (id: number, completed: boolean) => {
  const action = completed ? "complete" : "reopen";
  return request<{ ok: boolean }>(`goals/${id}/${action}`, "POST", {});
};
export const acceptGoal = (id: number, completed: boolean) =>
  request<{ ok: boolean }>(`goals/${id}/accept`, "POST", { completed });
export const deleteGoal = (id: number) =>
  request<{ ok: boolean }>(`goals/${id}`, "DELETE");
