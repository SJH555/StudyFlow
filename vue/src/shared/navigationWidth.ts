// 两级导航宽度使用应用设置表中的独立键。
export const navigationWidths = {
  primary: {
    settingKey: "primaryNavWidth",
    min: 72,
    max: 250,
  },
  secondary: {
    settingKey: "sectionNavWidth",
    min: 72,
    max: 240,
  },
} as const;

export type NavigationLevel = keyof typeof navigationWidths;

export function parseNavigationWidth(
  level: NavigationLevel,
  value: string | null | undefined,
): number | null {
  if (value == null || value.trim() === "") return null;
  const width = Number(value);
  const bounds = navigationWidths[level];
  if (!Number.isInteger(width) || width < bounds.min) return null;
  return Math.min(width, bounds.max);
}
