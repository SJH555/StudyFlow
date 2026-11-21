<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useSettingsStore } from "../../stores/settings";
import SectionSubnav from "../../shared/SectionSubnav.vue";
import BackupFiles from "./BackupFiles.vue";
import packageInfo from "../../../package.json";
const appVersion = packageInfo.version;
const settingsStore = useSettingsStore();
const {
  autostartEnabled,
  autostartBusy,
  settingsTab,
  resetTableList,
  resetTarget,
  logFiles,
  logDirectory,
  selectedLog,
  logContent,
  settingsPageHeading,
  dataDirectory,
  backupEnabled,
  backupDirectory,
  ringtoneVolume,
  startupWindow,
  closeBehavior,
} = storeToRefs(settingsStore);
const {
  showGeneral,
  setRingtoneVolume,
  toggleAutostart,
  chooseDataDirectory,
  showResetTables,
  showLogs,
  openLog,
  toggleAutoBackup,
} = settingsStore;

const navigationItems = [
  { key: "general", title: "常规", icon: "mdi-tune-variant" },
  { key: "storage", title: "数据与备份", icon: "mdi-database-outline" },
  { key: "logs", title: "日志", icon: "mdi-file-document-alert-outline" },
  { key: "reset", title: "重置数据", icon: "mdi-database-remove-outline" },
] as const;

function selectSection(key: string) {
  if (key === "general") void showGeneral();
  else if (key === "storage") settingsTab.value = "storage";
  else if (key === "logs") void showLogs();
  else if (key === "reset") void showResetTables();
}
</script>

<template>
  <VApp class="settings-app section-app">
    <div class="section-page-header">
      <div class="section-page-icon">
        <VIcon
          :icon="
            settingsTab === 'general'
              ? 'mdi-tune-variant'
              : settingsTab === 'storage'
                ? 'mdi-database-outline'
                : settingsTab === 'logs'
                  ? 'mdi-file-document-alert-outline'
                  : 'mdi-database-remove-outline'
          "
        />
      </div>
      <div>
        <span class="section-eyebrow">PREFERENCES</span>
        <h1>{{ settingsPageHeading.title }}</h1>
        <p>{{ settingsPageHeading.description }}</p>
      </div>
      <div class="section-page-actions">
        <VChip
          v-if="settingsTab === 'storage'"
          size="small"
          label
          variant="tonal"
          color="primary"
          prepend-icon="mdi-harddisk"
          >本地存储</VChip
        ><VBtn
          v-else-if="settingsTab === 'logs'"
          variant="tonal"
          color="primary"
          size="small"
          prepend-icon="mdi-refresh"
          @click="showLogs(true)"
          >刷新</VBtn
        >
      </div>
    </div>
    <div class="section-layout joined-panels">
      <SectionSubnav
        label="系统设置"
        label-icon="mdi-cog-outline"
        navigation-label="设置分类"
        :active-key="settingsTab"
        :items="navigationItems"
        @select="selectSection"
      />
      <div class="section-column">
        <VCard
          v-if="settingsTab === 'general'"
          class="section-card"
          variant="outlined"
          rounded="lg"
          elevation="0"
        >
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>开机后自动启动</strong
              ><small>开机后自动运行 StudyFlow。</small>
            </div>
            <VSwitch
              :model-value="autostartEnabled"
              color="primary"
              inset
              hide-details
              :loading="autostartBusy"
              :disabled="autostartBusy"
              aria-label="开机后自动启动"
              @update:model-value="toggleAutostart"
            />
          </div>
          <VDivider class="settings-row-divider" />
          <div class="section-setting-row settings-volume-row">
            <div class="section-setting-copy">
              <strong>铃声大小</strong
              ><small>提醒铃声的播放音量，拖动后立即生效；0 为静音。</small>
            </div>
            <div class="settings-volume-control">
              <VIcon
                :icon="
                  ringtoneVolume === 0
                    ? 'mdi-volume-off'
                    : ringtoneVolume < 50
                      ? 'mdi-volume-low'
                      : 'mdi-volume-high'
                "
                size="small"
              />
              <VSlider
                :model-value="ringtoneVolume"
                :min="0"
                :max="100"
                :step="1"
                color="#8F9CFA"
                hide-details
                aria-label="铃声大小"
                @update:model-value="setRingtoneVolume"
              />
              <span class="settings-volume-value">{{ ringtoneVolume }}</span>
            </div>
          </div>
          <VDivider class="settings-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>启动后窗口</strong
              ><small>控制应用启动时主窗口的显示方式。</small>
            </div>
            <VBtnToggle
              v-model="startupWindow"
              mandatory
              color="primary"
              variant="tonal"
              density="compact"
              aria-label="启动后窗口"
              ><VBtn value="show" prepend-icon="mdi-monitor">显示窗口</VBtn
              ><VBtn value="tray" prepend-icon="mdi-tray-arrow-down"
                >驻留托盘</VBtn
              ></VBtnToggle
            >
          </div>
          <VDivider class="settings-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>关闭按钮</strong
              ><small>控制点击窗口关闭按钮后的行为。</small>
            </div>
            <VBtnToggle
              v-model="closeBehavior"
              mandatory
              color="primary"
              variant="tonal"
              density="compact"
              aria-label="关闭按钮"
              ><VBtn value="tray" prepend-icon="mdi-minus">最小化</VBtn
              ><VBtn value="quit" prepend-icon="mdi-power"
                >退出</VBtn
              ></VBtnToggle
            >
          </div>
          <VDivider class="settings-row-divider" />
          <div class="section-setting-row">
            <div class="section-setting-copy">
              <strong>版本信息</strong><small>当前安装的 StudyFlow 版本</small>
            </div>
            <span class="settings-version-tag">v{{ appVersion }}</span>
          </div>
        </VCard>
        <VCard
          v-else-if="settingsTab === 'storage'"
          class="section-card"
          variant="outlined"
          rounded="lg"
          elevation="0"
        >
          <div class="settings-content-block">
            <div class="settings-block-heading">
              <strong>数据目录</strong
              ><small>数据库与备份随存储位置一起迁移。</small>
            </div>
            <div class="settings-path-row">
              <VTextField
                v-model="dataDirectory"
                aria-label="数据目录"
                variant="outlined"
                density="compact"
                hide-details
                readonly
                prepend-inner-icon="mdi-folder-outline"
              /><VBtn
                class="settings-choose-directory"
                variant="tonal"
                color="primary"
                rounded="lg"
                prepend-icon="mdi-folder-open-outline"
                @click="chooseDataDirectory"
                >选择目录</VBtn
              >
            </div>
            <VAlert
              type="info"
              variant="tonal"
              density="compact"
              icon="mdi-information-outline"
              >若目标目录已存在 study.db，迁移会停止，以免覆盖原有数据。</VAlert
            >
          </div>
          <div class="section-setting-row settings-backup-row">
            <div class="section-setting-copy">
              <strong>自动备份</strong
              ><small
                >开启后立即备份一次，此后每隔 2 天备份；每份保留 2 天。</small
              ><span class="settings-path-caption"
                >备份目录：{{ backupDirectory }}</span
              >
            </div>
            <VSwitch
              :model-value="backupEnabled"
              color="primary"
              inset
              hide-details
              aria-label="自动备份"
              @update:model-value="toggleAutoBackup"
            />
          </div>
          <BackupFiles />
        </VCard>
        <VCard
          v-else-if="settingsTab === 'logs'"
          class="section-card"
          variant="outlined"
          rounded="lg"
          elevation="0"
        >
          <div class="settings-log-location">
            <VIcon icon="mdi-folder-outline" size="small" /><span>{{
              logDirectory || "正在读取日志目录…"
            }}</span>
          </div>
          <VDivider />
          <VList class="settings-data-list" density="compact"
            ><VListItem
              v-for="file in logFiles"
              :key="file.name"
              :title="file.name"
              :subtitle="`${(file.size / 1024).toFixed(1)} KB · ${new Date(file.modified).toLocaleString('zh-CN')}`"
              prepend-icon="mdi-file-alert-outline"
              ><template #append
                ><VBtn
                  variant="tonal"
                  size="small"
                  color="primary"
                  @click="openLog(file.name)"
                  >查看</VBtn
                ></template
              ></VListItem
            ></VList
          >
          <div v-if="!logFiles.length" class="section-empty">
            <VIcon icon="mdi-check-circle-outline" /><b>最近没有错误日志</b
            ><span>出现错误或崩溃时，日志会显示在这里。</span>
          </div>
          <div v-if="selectedLog" class="settings-log-viewer">
            <div>
              <b>{{ selectedLog }}</b
              ><VBtn
                variant="text"
                size="small"
                @click="
                  selectedLog = '';
                  logContent = '';
                "
                >关闭</VBtn
              >
            </div>
            <pre>{{ logContent }}</pre>
          </div>
        </VCard>
        <VCard
          v-else-if="settingsTab === 'reset'"
          class="section-card"
          variant="outlined"
          rounded="lg"
          elevation="0"
        >
          <div class="settings-reset-note">
            <VAlert
              type="warning"
              variant="tonal"
              density="compact"
              icon="mdi-alert-outline"
              >删除后无法恢复。学习项目若仍关联计划或学习记录，请先重置关联表。</VAlert
            >
          </div>
          <VList class="settings-data-list" density="compact"
            ><VListItem
              v-for="table in resetTableList"
              :key="table.name"
              :title="`${table.label}（${table.name}）`"
              :subtitle="`${table.count} 条数据`"
              prepend-icon="mdi-table"
              ><template #append
                ><VBtn
                  variant="tonal"
                  color="error"
                  size="small"
                  :disabled="table.count === 0"
                  @click="resetTarget = table"
                  >永久删除</VBtn
                ></template
              ></VListItem
            ></VList
          >
          <div v-if="!resetTableList.length" class="section-empty">
            <VIcon icon="mdi-database-off-outline" /><b>没有可重置的数据表</b>
          </div>
        </VCard>
      </div>
    </div>
  </VApp>
</template>

<style scoped>
.settings-app .section-column > .section-card.v-card {
  padding-inline: calc(var(--space-6) + var(--space-5));
}

@media (max-width: 1100px) {
  .settings-app .section-column > .section-card.v-card {
    padding-inline: var(--space-6);
  }
}

.section-page-header > .section-page-actions {
  display: flex;
  align-items: center;
  flex: none;
  margin-left: auto;
}

.settings-row-divider.v-divider {
  border-color: #f0f3fa;
  opacity: 0.8;
}
.settings-content-block :deep(.v-alert) {
  border-radius: 9px;
}
.settings-backup-row {
  min-height: 122px;
  padding: 23px 0;
}
@media (max-width: 1100px) {
  .settings-path-row {
    flex-wrap: wrap;
  }
}
.settings-content-block {
  border-bottom: 1px solid var(--outline-card);
}
.settings-path-row {
  margin-top: 24px;
  margin-bottom: 21px;
}
.settings-path-row :deep(.v-field) {
  min-height: 36px;
  height: 36px;
}
.settings-path-row :deep(.v-field__input) {
  min-height: 36px;
  padding-top: 0;
  padding-bottom: 0;
}
.settings-backup-row :deep(.v-switch) {
  margin-left: auto;
}
.settings-path-row :deep(.v-text-field) {
  margin-right: auto;
}
.settings-path-row :deep(.v-btn) {
  height: 36px;
  margin-left: auto;
}
.settings-path-row :deep(.v-field) {
  border-radius: 8px;
  color: var(--ink);
}
.settings-path-row :deep(.v-field .v-icon) {
  color: var(--purple);
}
.settings-path-row :deep(.v-field) {
  background: var(--surface-hidden) !important;
  box-shadow: none !important;
}
.settings-path-row :deep(.v-field--focused) {
  background: var(--surface-hidden) !important;
  box-shadow: none !important;
}
.settings-path-row :deep(.v-field--active) {
  background: var(--surface-hidden) !important;
  box-shadow: none !important;
}
.settings-path-row :deep(.v-field__outline) {
  color: var(--outline-card) !important;
  --v-field-border-opacity: 1;
}
.settings-path-row :deep(.v-field--focused .v-field__outline) {
  color: var(--outline-card) !important;
  --v-field-border-opacity: 1;
}
.settings-path-row :deep(.v-text-field .v-field__input) {
  padding-top: 0 !important;
}

.settings-volume-control {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  max-width: 340px;
  min-width: 220px;
  color: var(--muted);
}
.settings-volume-control :deep(.v-slider) {
  flex: 1;
}
.settings-volume-value {
  min-width: 30px;
  text-align: right;
  font-size: 13px;
  font-weight: 650;
  color: var(--ink);
  font-variant-numeric: tabular-nums;
}
.settings-block-heading strong {
  font-size: 14px;
  line-height: 1.35;
  font-weight: 650;
}
.settings-block-heading small {
  color: var(--section-muted);
  font-size: 12px;
  line-height: 1.45;
}
.settings-content-block {
  padding-block: var(--space-5);
}
.settings-block-heading {
  display: grid;
  gap: 4px;
  margin-bottom: 14px;
}
.settings-path-row {
  justify-content: space-between;
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
}
.settings-path-row :deep(.v-text-field) {
  min-width: 0;
  flex: 1;
}
.settings-path-row :deep(.v-btn) {
  flex: none;
}
.settings-content-block :deep(.v-alert) {
  margin-top: 14px;
  font-size: 12px;
  line-height: 1.45;
}
.settings-path-caption {
  margin-top: 2px;
  color: var(--section-muted);
  font-size: 11px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}
.settings-log-location {
  display: flex;
  align-items: center;
  gap: 9px;
  padding-block: var(--space-3);
  color: var(--section-muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.settings-log-location :deep(.v-icon) {
  flex: none;
  color: var(--section-primary);
}
.settings-data-list.v-list {
  padding: 0;
  background: transparent;
}
.settings-data-list :deep(.v-list-item) {
  min-height: 68px;
  padding-inline: 0;
  border-bottom: 1px solid var(--section-outline);
}
.settings-data-list :deep(.v-list-item:last-child) {
  border-bottom: 0;
}
.settings-data-list :deep(.v-list-item-title) {
  font-size: 14px;
  font-weight: 600;
}
.settings-data-list :deep(.v-list-item-subtitle) {
  font-size: 12px;
}
.settings-data-list :deep(.v-list-item__prepend > .v-icon) {
  color: var(--section-primary);
  opacity: 1;
}
.settings-log-viewer {
  margin-block: var(--space-3) var(--space-5);
  border: 1px solid var(--section-outline);
  border-radius: 10px;
  overflow: hidden;
}
.settings-log-viewer > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: var(--surface-compare);
  font-size: 13px;
}
.settings-log-viewer pre {
  max-height: 340px;
  margin: 0;
  padding: 13px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.5;
}
.settings-reset-note {
  padding-block: var(--space-4) var(--space-2);
}
.settings-reset-note :deep(.v-alert) {
  font-size: 12px;
  line-height: 1.45;
}
@media (max-width: 960px) {
  .settings-path-row {
    flex-wrap: wrap;
  }
}
@media (max-width: 960px) {
  .settings-path-row :deep(.v-text-field) {
    flex-basis: 100%;
  }
}

.settings-version-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 30px;
  padding: 4px 11px;
  border-radius: var(--shape-small);
  background: var(--surface-version-tag);
  color: var(--ink);
  font-size: var(--text-body);
  font-weight: var(--weight-strong);
  white-space: nowrap;
}
</style>
