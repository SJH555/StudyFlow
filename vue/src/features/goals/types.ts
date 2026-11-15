// 阶段目标 API 的数据契约；截止日期始终表示本地自然日。
export interface Goal {
  id: number;
  title: string;
  /** 截止日期，格式为 YYYY-MM-DD。 */
  deadline: string;
  note: string;
  completed: boolean;
  accepted: boolean;
  acceptedAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface GoalInput {
  /** 新建时省略，编辑时提供。 */
  id?: number;
  title: string;
  deadline: string;
  note: string;
}
