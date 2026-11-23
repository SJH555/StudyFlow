import type { Plan } from "../api";

export function dateKey(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const shanghaiDay = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" });

export function nextEffectiveDay(now = new Date()) {
  const tomorrow = new Date(now.getTime() + 86_400_000);
  const parts = shanghaiDay.formatToParts(tomorrow);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((item) => item.type === type)?.value;
  return `${part("year")}-${part("month")}-${part("day")}`;
}

export function plannedStudyMinutes(p: Plan) {
  // 有效学习时长 = 计划时长 - 休息时长，最小为 0。
  return Math.max(0, p.duration - (p.breakMinutes || 0));
}

export function clockLabel(seconds: number) {
  const h = Math.floor(seconds / 3600),
    m = Math.floor((seconds % 3600) / 60),
    ss = seconds % 60;
  return h
    ? `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`
    : `${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
}

export function fmt(sec: number) {
  const h = Math.floor(sec / 3600),
    m = Math.floor((sec % 3600) / 60);
  return h ? `${h}小时${m ? ` ${m}分` : ""}` : `${m}分钟`;
}

export function formatPlanMinutes(minutes: number) {
  const hours = Math.floor(minutes / 60),
    remaining = minutes % 60;
  return hours
    ? `${hours}小时${remaining ? `${remaining}分钟` : ""}`
    : `${remaining}分钟`;
}

export function reviewGap(
  plannedSeconds: number,
  actualSeconds: number,
  threshold: number,
) {
  // 按“目标比例”计算还差的分钟数：至少 1 分钟，向上取整。
  return formatPlanMinutes(
    Math.max(
      1,
      Math.ceil(
        Math.max(0, (plannedSeconds * threshold) / 100 - actualSeconds) / 60,
      ),
    ),
  );
}

// 将分钟数转成“HH:MM”，超过 1440（一天）时标记为“次日”。
export function formatPlanClock(minutes: number) {
  const clock = `${String(Math.floor(minutes / 60) % 24).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
  return minutes >= 1440 ? `次日 ${clock}` : clock;
}

export function time(date: string) {
  return new Date(date).toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function planEnd(p: Plan) {
  const [h, m] = p.startTime.split(":").map(Number),
    d = new Date();
  d.setHours(h, m + p.duration, 0, 0);
  return d.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

export function previewPlanNote(note: string, maxCharacters = 14) {
  const plain = note.replace(/\s+/g, " "),
    characters = Array.from(plain);
  return characters.length > maxCharacters
    ? characters.slice(0, maxCharacters).join("") + "…"
    : plain;
}

export function formatDateChinese(value: string) {
  const d = new Date(value + "T12:00:00");
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`;
}

// 把日期选择器回传的值统一规范为 "YYYY-MM-DD"；无效输入返回 null。
// 兼容三种来源：已是日期串、可解析的字符串、时间戳（Date 或 number）。
export function parseDateKey(value: unknown) {
  if (!value) return null;
  const day =
    typeof value === "string" && /^\d{4}-\d{2}-\d{2}/.test(value)
      ? value.slice(0, 10)
      : dateKey(new Date(value as string | number));
  return /^\d{4}-\d{2}-\d{2}$/.test(day) ? day : null;
}

// 按自然日剩余数显示截止状态，避免各页面出现不同文案。
export function formatRemainingDays(days: number) {
  if (days < 0) return `已逾期 ${Math.abs(days)} 天`;
  if (days === 0) return "今天截止";
  return `还剩 ${days} 天`;
}
