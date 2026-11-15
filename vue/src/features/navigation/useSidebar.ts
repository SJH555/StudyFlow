import { computed, onUnmounted, ref, watch } from "vue";
import { navigationWidths, parseNavigationWidth } from "../../shared/navigationWidth";
import { useSettingsStore } from "../../stores/settings";

// 侧边导航：定义导航项并管理侧边栏宽度（拖拽/键盘调整及设置持久化）。
export function useSidebar() {
  const settingsStore = useSettingsStore();
  const primaryNavigation = [
    { key: "today", label: "今日计划", icon: "mdi-calendar-today-outline" },
    {
      key: "subjects",
      label: "学习项目",
      icon: "mdi-book-open-page-variant-outline",
    },
    { key: "plans", label: "学习计划", icon: "mdi-calendar-clock-outline" },
    { key: "stats", label: "学习统计", icon: "mdi-chart-bar" },
    { key: "summary", label: "学情追踪", icon: "mdi-chart-timeline-variant" },
    { key: "activity", label: "活动提醒", icon: "mdi-bell-outline" },
  ] as const;

  // 侧边栏可调范围 72–250px；低于 180px 时折叠为仅图标。
  const sidebarMinWidth = navigationWidths.primary.min,
    sidebarMaxWidth = navigationWidths.primary.max,
    sidebarCompactWidth = 180;

  function clampSidebarWidth(width: number) {
    return Math.min(
      sidebarMaxWidth,
      Math.max(sidebarMinWidth, Math.round(width)),
    );
  }

  const sidebarWidth = ref<number>(sidebarMaxWidth),
    sidebarCompact = computed(() => sidebarWidth.value < sidebarCompactWidth);

  watch(
    () => settingsStore.settings.primaryNavWidth,
    (value) => {
      const saved = parseNavigationWidth("primary", value);
      if (saved !== null) sidebarWidth.value = saved;
    },
    { immediate: true },
  );

  let sidebarResizing = false;

  function saveSidebarWidth() {
    void settingsStore.saveNavigationWidth("primary", sidebarWidth.value);
  }

  function beginSidebarResize(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    sidebarResizing = true;
    document.body.classList.add("sidebar-resizing");
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function moveSidebarResize(event: PointerEvent) {
    if (!sidebarResizing) return;
    event.preventDefault();
    sidebarWidth.value = clampSidebarWidth(event.clientX - 8);
  }

  function endSidebarResize(event: PointerEvent) {
    if (!sidebarResizing) return;
    sidebarResizing = false;
    document.body.classList.remove("sidebar-resizing");
    const handle = event.currentTarget as HTMLElement;
    if (handle.hasPointerCapture(event.pointerId))
      handle.releasePointerCapture(event.pointerId);
    saveSidebarWidth();
  }

  function resizeSidebarByKey(event: KeyboardEvent) {
    let next = sidebarWidth.value;
    if (event.key === "ArrowLeft") next -= 10;
    else if (event.key === "ArrowRight") next += 10;
    else if (event.key === "Home") next = sidebarMinWidth;
    else if (event.key === "End") next = sidebarMaxWidth;
    else return;
    event.preventDefault();
    sidebarWidth.value = clampSidebarWidth(next);
    saveSidebarWidth();
  }

  onUnmounted(() => document.body.classList.remove("sidebar-resizing"));
  return {
    primaryNavigation,
    sidebarMinWidth,
    sidebarMaxWidth,
    sidebarWidth,
    sidebarCompact,
    beginSidebarResize,
    moveSidebarResize,
    endSidebarResize,
    resizeSidebarByKey,
  };
}
