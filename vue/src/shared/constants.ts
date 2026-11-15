// 全局 UI 常量：项目预设配色与时间选择项，跨模块复用。

// 项目默认配色：新建科目/计划时按顺序取色，避免相邻项颜色重复。
export const colorPalette = [
  "#007AFF",
  "#34C759",
  "#FF9500",
  "#AF52DE",
  "#5AC8FA",
  "#FF3B30",
  "#5856D6",
  "#FF2D55",
  "#A2845E",
  "#00C7BE",
];

export const weekNames = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];

export const hourOptions = Array.from({ length: 24 }, (_, hour) => ({
  title: `${String(hour).padStart(2, "0")} 时`,
  value: String(hour).padStart(2, "0"),
}));

export const minuteOptions = Array.from({ length: 60 }, (_, minute) => ({
  title: `${String(minute).padStart(2, "0")} 分`,
  value: String(minute).padStart(2, "0"),
}));
