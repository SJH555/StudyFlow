// 统计图与概览卡片共用配色，柱状图的高亮描边在此维护。

export function chartSeriesColors(colors: Record<string, string>) {
  const tokens = getComputedStyle(document.documentElement);
  const plan = tokens.getPropertyValue("--color-plan-learning").trim() || colors.primary;
  const actual = tokens.getPropertyValue("--color-actual-learning").trim() || colors.warning;
  return { plan, actual, trend: actual };
}

export const chartTheme = {
  planBarHighlight: "#396ac4",
  actualBarHighlight: "#aa6723",

  // 趋势图坐标轴与网格线。
  axisLine: "#e8e6e0",
  axisLabel: "#89877f",
  splitLine: "#efeee9",

  // 项目对比图（横向条形图）专用配色。
  projectAxisLabel: "#8a90a0",
  projectSplitLine: "#edf0f5",
  projectCategoryLabel: "#3c4454",
  projectValueLabel: "#515b70",
};
