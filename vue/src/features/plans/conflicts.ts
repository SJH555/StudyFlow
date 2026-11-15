import type { Plan } from "../../api";

// 计划时间冲突检测：把“星期几 + 开始时刻”统一换算成分钟数，
// 再与现有计划做区间重叠判断（跨周时把相邻计划偏移 ±一周再比较）。
export function conflictingPlan(
  candidate: Plan,
  plans: Plan[],
  weekNames: readonly string[],
) {
  const start = (time: string) =>
    Number(time.slice(0, 2)) * 60 + Number(time.slice(3, 5));
  const days = (value: string) => value.split(",").map(Number);
  const week = 7 * 24 * 60; // 一周分钟数，用于把相邻周的计划对齐到同一周比较
  for (const existing of plans) {
    if (!existing.active || existing.id === candidate.id) continue;
    for (const day of days(candidate.weekdays))
      for (const otherDay of days(existing.weekdays))
        for (const shift of [-week, 0, week]) {
          const from = day * 1440 + start(candidate.startTime),
            otherFrom = otherDay * 1440 + start(existing.startTime) + shift;
          if (
            from < otherFrom + existing.duration &&
            otherFrom < from + candidate.duration
          )
            return `${weekNames[day]}的时间与“${existing.subject}” ${existing.startTime} 开始的计划冲突，请调整时间或重复星期`;
        }
  }
  return "";
}
