// 图表主题：坐标轴等辅助色集中管理，系列色与 Vuetify 主题保持一致。

export function chartSeriesColors(
  colors: Record<string, string>,
  planBorderColor?: string,
) {
  return {
    plan: planBorderColor || colors.primary,
    actual: colors.success,
    trend: colors.success,
  };
}

export const chartTheme = {
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
