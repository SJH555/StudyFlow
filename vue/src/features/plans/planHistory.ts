import type { Plan } from "../../api";

type StatusChange = {
  day: string;
  status: "stopped" | "deleted";
};

type PlanHistoryEntry = {
  version: Plan;
  statusChanges: StatusChange[];
};

export function samePlanContent(left: Plan, right: Plan) {
  return left.subjectId === right.subjectId
    && left.startTime === right.startTime
    && left.duration === right.duration
    && left.breakMinutes === right.breakMinutes
    && [...new Set(left.weekdays.split(","))].sort().join(",")
      === [...new Set(right.weekdays.split(","))].sort().join(",")
    && (left.colorOverride ?? left.color) === (right.colorOverride ?? right.color)
    && left.note === right.note;
}

export function buildPlanHistory(versions: Plan[]): PlanHistoryEntry[] {
  // 接口按日期和保存顺序倒排，每个生效日只取最后一次保存。
  const effective = versions.filter(
    (version, index) => index === 0 || version.effectiveDay !== versions[index - 1].effectiveDay,
  );
  const entries: PlanHistoryEntry[] = [];
  let current: PlanHistoryEntry | undefined;
  let status: Plan["status"];

  for (const version of effective.reverse()) {
    if (!current || !samePlanContent(current.version, version)) {
      current = { version, statusChanges: [] };
      entries.push(current);
    }
    const nextStatus = version.status;
    if ((nextStatus === "stopped" || nextStatus === "deleted")
      && nextStatus !== status && version.effectiveDay) {
      current.statusChanges.push({ day: version.effectiveDay, status: nextStatus });
    }
    status = nextStatus;
  }
  return entries.reverse();
}
