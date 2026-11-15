<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { navigationWidths } from "./navigationWidth";
import { useSettingsStore } from "../stores/settings";

export interface SectionNavItem {
  key: string;
  title: string;
  icon: string;
}

defineProps<{
  label: string;
  labelIcon: string;
  navigationLabel: string;
  activeKey: string;
  items: readonly SectionNavItem[];
}>();

const emit = defineEmits<{
  select: [key: string];
}>();

// 与主导航一致，72px 时导航项只显示图标；展开宽度上限为 240px。
const minWidth = navigationWidths.secondary.min;
const maxWidth = navigationWidths.secondary.max;
const settingsStore = useSettingsStore();
const nav = ref<HTMLElement | null>(null);
const width = ref<number>(minWidth);
let resizing = false;

function clampWidth(value: number) {
  return Math.min(maxWidth, Math.max(minWidth, Math.round(value)));
}

function setWidth(value: number) {
  width.value = clampWidth(value);
  updateNavVariables();
  nav.value?.parentElement?.style.setProperty(
    "--section-nav-width",
    `${width.value}px`,
  );
}

function updateNavVariables() {
  const progress = (width.value - minWidth) / (maxWidth - minWidth);
  const iconGap = Math.min(13, Math.max(0, (width.value - 130) * 0.08));
  const appendWidth = Math.min(22, Math.max(0, (width.value - 120) * 0.4));
  nav.value?.style.setProperty("--section-nav-width", `${width.value}px`);
  nav.value?.style.setProperty(
    "--section-nav-padding",
    `${3 + 9 * progress}px`,
  );
  nav.value?.style.setProperty(
    "--section-label-padding",
    `${3 + 11 * progress}px`,
  );
  nav.value?.style.setProperty(
    "--section-item-start-padding",
    `${20.5 - 6.5 * progress}px`,
  );
  nav.value?.style.setProperty(
    "--section-item-end-padding",
    `${4 + 10 * progress}px`,
  );
  nav.value?.style.setProperty("--section-icon-gap", `${iconGap}px`);
  nav.value?.style.setProperty("--section-append-width", `${appendWidth}px`);
  nav.value?.style.setProperty(
    "--section-text-width",
    `${Math.min(180, (width.value - minWidth) * 1.1)}px`,
  );
}

onMounted(() => {
  const saved = settingsStore.getNavigationWidth("secondary");
  if (saved !== null) {
    setWidth(saved);
    return;
  }
  setWidth(nav.value?.getBoundingClientRect().width ?? minWidth);
});

function beginResize(event: PointerEvent) {
  if (event.button !== 0) return;
  event.preventDefault();
  resizing = true;
  document.body.classList.add("section-nav-resizing");
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
}

function moveResize(event: PointerEvent) {
  if (!resizing || !nav.value) return;
  event.preventDefault();
  setWidth(event.clientX - nav.value.getBoundingClientRect().left);
}

function endResize(event: PointerEvent) {
  if (!resizing) return;
  resizing = false;
  document.body.classList.remove("section-nav-resizing");
  const handle = event.currentTarget as HTMLElement;
  if (handle.hasPointerCapture(event.pointerId)) {
    handle.releasePointerCapture(event.pointerId);
  }
  saveWidth();
}

function saveWidth() {
  void settingsStore.saveNavigationWidth("secondary", width.value);
}

function resizeByKey(event: KeyboardEvent) {
  let next = width.value;
  if (event.key === "ArrowLeft") next -= 10;
  else if (event.key === "ArrowRight") next += 10;
  else if (event.key === "Home") next = minWidth;
  else if (event.key === "End") next = maxWidth;
  else return;
  event.preventDefault();
  setWidth(next);
  saveWidth();
}

onUnmounted(() => {
  document.body.classList.remove("section-nav-resizing");
});
</script>

<template>
  <nav
    ref="nav"
    class="section-subnav section-subnav--resizable"
    :aria-label="navigationLabel"
  >
    <span class="section-nav-label" :aria-label="label">
      <VIcon :icon="labelIcon" aria-hidden="true" />
      <span class="section-nav-label-text">{{ label }}</span>
      <VTooltip v-if="width < 150" activator="parent" location="end">
        {{ label }}
      </VTooltip>
    </span>
    <VList density="compact" nav>
      <template v-for="(item, index) in items" :key="item.key">
        <VDivider v-if="index > 0" class="section-nav-divider" />
        <VListItem
          :title="item.title"
          :prepend-icon="item.icon"
          :append-icon="activeKey === item.key ? 'mdi-chevron-right' : undefined"
          :active="activeKey === item.key"
          color="primary"
          rounded="lg"
          @click="emit('select', item.key)"
        >
          <VTooltip v-if="width < 150" activator="parent" location="end">
            {{ item.title }}
          </VTooltip>
        </VListItem>
      </template>
    </VList>
    <div
      class="nav-resize-handle section-nav-resize-handle"
      role="separator"
      aria-label="调整二级导航宽度"
      aria-orientation="vertical"
      :aria-valuenow="width"
      :aria-valuemin="minWidth"
      :aria-valuemax="maxWidth"
      tabindex="0"
      @pointerdown.stop="beginResize"
      @pointermove.stop="moveResize"
      @pointerup.stop="endResize"
      @pointercancel.stop="endResize"
      @click.stop.prevent
      @keydown="resizeByKey"
    />
  </nav>
</template>

<style scoped>
.nav-resize-handle.section-nav-resize-handle::after {
  inset-inline-start: 8px;
}
.settings-app .section-nav-label {
  display: block;
  margin: 0 0 10px;
  padding: 12px 14px;
  border-radius: 9px;
  color: var(--muted);
  font-size: 14px;
  font-weight: 650;
  letter-spacing: 0;
  background: rgb(248 248 249);
}

body.section-nav-resizing .section-nav-resize-handle::after {
  background: var(--nav-resize-handle-active);
}

.settings-app .section-subnav :deep(.v-list) {
  display: grid;
  gap: 5px;
  padding: 0;
}
.settings-app .section-subnav :deep(.v-list-item) {
  position: relative;
  min-height: 76px;
  margin: 0;
  padding-inline: 14px;
}
.settings-app .section-subnav :deep(.v-list-item-subtitle) {
  line-height: 1.4;
  opacity: 0.8;
}
.settings-app .section-subnav :deep(.v-list-item__prepend > .v-icon) {
  margin-inline-end: 13px;
  font-size: 23px;
}
.settings-app .section-subnav :deep(.v-list-item--active::before) {
  content: "";
  position: absolute;
  z-index: 2;
  top: 10px;
  bottom: 10px;
  left: 0;
  width: 3px;
  border-radius: 3px;
  background: var(--purple);
}
@media (max-width: 950px) {
  .settings-app .section-subnav :deep(.v-list) {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
@media (max-width: 950px) {
  .settings-app .section-subnav :deep(.v-list-item) {
    min-height: 64px;
  }
}
@media (max-width: 950px) {
  .settings-app .section-subnav :deep(.v-list-item-subtitle) {
    display: none;
  }
}
.settings-app .section-subnav :deep(.v-list-item__spacer) {
  width: 0 !important;
  min-width: 0 !important;
}
.settings-app .section-subnav :deep(.v-list-item__content) {
  min-width: 0;
}
.settings-app .section-subnav :deep(.v-list-item-title) {
  font-size: 16px;
  font-weight: 650;
}
.settings-app .section-subnav :deep(.v-list-item-subtitle) {
  margin-top: 3px;
  font-size: 13px;
}
.settings-app .section-subnav :deep(.v-list-item:not(.v-list-item--active)) {
  background: #fff;
  color: #000;
}
.settings-app
  .section-subnav
  :deep(.v-list-item:not(.v-list-item--active) .v-list-item-title) {
  color: #000;
}
.section-subnav .section-nav-divider {
  margin-inline: 14px;
  border-color: var(--section-outline);
  opacity: 1;
}
.settings-app
  .section-subnav
  :deep(.v-list-item:not(.v-list-item--active) .v-list-item-subtitle) {
  color: var(--section-muted);
  opacity: 1;
}

.section-subnav {
  padding: 12px 8px 8px;
  border: 1px solid var(--section-outline);
  border-radius: 14px;
  background: var(--section-surface);
}
.section-nav-label {
  display: block;
  padding: 0 10px 8px;
  color: var(--section-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.04em;
}
.section-subnav :deep(.v-list) {
  padding: 0;
  background: transparent;
}
.section-subnav :deep(.v-list-item) {
  min-height: 56px;
  margin: 2px 0;
  letter-spacing: 0;
}
.section-subnav :deep(.v-list-item-title) {
  font-size: 14px;
  font-weight: 650;
}
.section-subnav :deep(.v-list-item-subtitle) {
  font-size: 11px;
  line-height: 1.3;
  opacity: 0.75;
}
.section-subnav :deep(.v-list-item__prepend > .v-icon) {
  margin-inline-end: 12px;
  opacity: 1;
}
.section-subnav :deep(.v-list-item--active) {
  background: var(--surface-hidden);
  color: var(--section-primary);
}
@media (max-width: 1180px) {
  .section-subnav {
    padding: 8px 10px;
  }
}
@media (max-width: 1180px) {
  .section-nav-label {
    padding: 0 6px 4px;
  }
}
@media (max-width: 1180px) {
  .section-subnav :deep(.v-list) {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
}
@media (max-width: 1180px) {
  .section-subnav :deep(.v-list-item) {
    flex: 0 1 auto;
    min-height: 44px;
    margin: 0;
  }
}
@media (max-width: 1180px) {
  .section-subnav :deep(.v-list-item-subtitle) {
    display: none;
  }
}

.section-subnav {
  position: relative;
  min-inline-size: 0;
  padding-inline: var(--section-nav-padding, 12px) !important;
  overflow: hidden;
}

.section-nav-label {
  overflow: clip;
  white-space: nowrap;
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  .section-nav-label {
  padding-inline: var(--section-label-padding, 14px);
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  :deep(.v-list-item) {
  padding-inline-start: var(--section-item-start-padding, 14px);
  padding-inline-end: var(--section-item-end-padding, 14px);
  overflow: clip;
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  :deep(.v-list-item__prepend > .v-icon) {
  margin-inline-end: var(--section-icon-gap, 13px);
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  :deep(.v-list-item__content) {
  flex: 0 1 auto;
  min-inline-size: 0;
  max-inline-size: var(--section-text-width, 180px);
  overflow: clip;
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  :deep(.v-list-item-title) {
  overflow: hidden;
  text-overflow: clip;
  white-space: nowrap;
}

.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  :deep(.v-list-item__append) {
  flex: 0 1 auto;
  min-inline-size: 0;
  max-inline-size: var(--section-append-width, 22px);
  overflow: clip;
}

.section-nav-resize-handle {
  inset-inline-end: 0;
}

@media (max-width: 950px) {
  .section-subnav {
    padding-inline: 12px !important;
  }

  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    :deep(.v-list-item) {
    padding-inline-start: 14px;
    padding-inline-end: 14px;
  }

  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    .section-nav-label {
    padding-inline: 14px;
  }

  :where(.review-app).settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    .section-nav-label {
    padding-inline: 7px;
  }

  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    :deep(.v-list-item__prepend > .v-icon) {
    margin-inline-end: 13px;
  }

  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    :deep(.v-list-item__content) {
    max-inline-size: none;
  }

  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    :deep(.v-list-item__append) {
    max-inline-size: none;
  }

  .section-nav-resize-handle {
    display: none;
  }
}
.settings-app .section-subnav {
  padding: 12px;
  border: 1px solid var(--outline-card);
}
@media (max-width: 950px) {
  .settings-app .section-subnav {
    min-height: 0;
  }
}
.settings-app .section-subnav {
  border-top: 0;
  background: #fff;
  border-radius: 8px 2px 2px 0;
}
@media (max-width: 950px) {
  .settings-app .section-subnav {
    border-radius: 8px 8px 2px 2px;
  }
}
.settings-app .section-subnav {
  box-shadow: none;
}
.joined-panels > .section-subnav {
  border-right: 1px solid var(--joined-panel-seam-color);
  border-radius: var(--joined-panel-radius) var(--joined-panel-seam-radius)
    var(--joined-panel-seam-radius) 0;
}
.settings-app .joined-panels > .section-subnav {
  border-top: 0;
  border-bottom: 0;
}

:where(.review-app) .section-subnav {
  min-height: calc(100vh - 185px);
  background: #ffffffdc;
  border-color: var(--review-line);
  border-radius: 16px;
  box-shadow: 0 6px 28px #274d9a0b;
}

:where(.review-app) .section-nav-label {
  color: var(--review-ink);
  font-weight: 700;
  letter-spacing: 0;
}

:where(.review-app) .section-subnav :deep(.v-list-item__spacer) {
  width: 0 !important;
  min-width: 0 !important;
}

:where(.review-app) .section-subnav :deep(.v-list-item__content) {
  min-width: 0;
}

:where(.review-app) .section-subnav :deep(.v-list-item-subtitle) {
  font-size: 11px;
  color: var(--review-primary);
  opacity: 1;
}

:where(.review-app) .section-subnav :deep(.v-list-item__append > .v-icon) {
  opacity: 1;
}

:where(.review-app) .section-subnav :deep(.v-list-item--active) {
  background: #e3ebff;
  color: var(--review-primary);
}

:where(.review-app) .section-subnav {
  padding: 13px 7px;
}

:where(.review-app) .section-nav-label {
  padding: 0 7px 7px;
  font-size: 14px;
}

:where(.review-app) .section-subnav :deep(.v-list-item) {
  min-height: 46px;
  padding-inline: 6px;
}

:where(.review-app) .section-subnav :deep(.v-list-item-title) {
  font-size: 13px;
}

:where(.review-app) .section-subnav :deep(.v-list-item__prepend > .v-icon) {
  font-size: 19px;
  margin-inline-end: 5px;
}

:where(.review-app) .section-subnav :deep(.v-list-item__append > .v-icon) {
  font-size: 15px;
  margin-inline-start: 3px;
}

@media (max-width: 950px) {
  :where(.review-app) .section-subnav {
    min-height: 0;
  }
}

:where(.review-app).settings-app .joined-panels > .section-subnav {
  min-height: 0;
  padding: 12px;
  border: 1px solid var(--outline-card);
  border-top: 0;
  border-bottom: 0;
  border-right: 1px solid var(--joined-panel-seam-color);
  border-radius: 8px var(--joined-panel-seam-radius)
    var(--joined-panel-seam-radius) 0;
  background: #fff;
  box-shadow: none;
}

:where(.review-app).settings-app .section-subnav :deep(.v-list-item--active) {
  background: var(--surface-hidden);
  color: var(--section-primary);
}

:where(.review-app).settings-app
  .section-subnav
  :deep(.v-list-item--active .v-list-item-subtitle) {
  color: inherit;
  opacity: 0.8;
}

@media (max-width: 950px) {
  :where(.review-app).settings-app .joined-panels > .section-subnav {
    border-right: 0;
    border-bottom: 1px solid var(--outline-card);
    border-radius: 8px 8px var(--joined-panel-seam-radius)
      var(--joined-panel-seam-radius);
  }
}
.settings-app
  .section-layout
  .section-subnav.section-subnav--resizable
  .section-nav-label {
  display: flex;
  align-items: center;
  gap: var(--section-icon-gap, 13px);
  padding-inline-start: var(--section-item-start-padding, 14px);
  padding-inline-end: var(--section-item-end-padding, 14px);
}

.section-nav-label > .v-icon {
  flex: none;
  font-size: 23px;
  color: inherit;
}

.section-nav-label-text {
  min-width: 0;
  max-width: var(--section-text-width, 180px);
  overflow: clip;
  white-space: nowrap;
}

@media (max-width: 950px) {
  .settings-app
    .section-layout
    .section-subnav.section-subnav--resizable
    .section-nav-label {
    gap: 13px;
    padding-inline: 14px;
  }

  .section-nav-label-text {
    max-width: none;
  }
}
</style>
