<script setup lang="ts">
import { onScopeDispose, ref } from "vue";
import { Application } from "@wailsio/runtime";
import { request } from "../../api";
import { useAppStore } from "../../stores/app";

type Preview = {
  id: string;
  filename: string;
  exportedAt?: string;
  version: number;
  targetVersion: number;
  needsUpgrade: boolean;
  legacy: boolean;
  subjects: number;
  sessions: number;
  timezoneNote?: string;
};

const appStore = useAppStore();
const busy = ref<"export" | "preview" | "restore" | "restart">();
const preview = ref<Preview | null>(null);
const restored = ref(false);
const restoreError = ref("");
let disposed = false;

async function exportData() {
  busy.value = "export";
  try {
    const path = await request<string>("data-transfer/export", "POST");
    if (path) appStore.notifyTop("已导出备份文件");
  } catch (error) {
    appStore.setError(error);
  } finally {
    busy.value = undefined;
  }
}

async function chooseBackup() {
  busy.value = "preview";
  try {
    const candidate = await request<Preview | null>("data-transfer/preview", "POST");
    if (disposed && candidate) {
      await request("data-transfer/cancel", "POST", { id: candidate.id });
    } else {
      preview.value = candidate;
    }
  } catch (error) {
    if (!disposed) appStore.setError(error);
  } finally {
    busy.value = undefined;
  }
}

async function closePreview() {
  const id = preview.value?.id;
  preview.value = null;
  const failed = !!restoreError.value;
  restoreError.value = "";
  if (id && !failed) {
    await request("data-transfer/cancel", "POST", { id }).catch(appStore.setError);
  }
}

async function restoreData() {
  if (!preview.value || restoreError.value || busy.value) return;
  busy.value = "restore";
  try {
    await request("data-transfer/restore", "POST", { id: preview.value.id });
    restored.value = true;
  } catch (error) {
    restoreError.value = error instanceof Error ? error.message : String(error);
  } finally {
    busy.value = undefined;
  }
}

async function reopen() {
  busy.value = "restart";
  try {
    await Application.Quit();
  } catch (error) {
    restoreError.value = error instanceof Error ? error.message : String(error);
    busy.value = undefined;
  }
}

onScopeDispose(() => {
  disposed = true;
  if (preview.value && !busy.value && !restored.value) void closePreview();
});
</script>

<template>
  <section class="backup-files" aria-labelledby="backup-files-title">
    <div class="backup-file-row">
      <div class="section-setting-copy">
        <h2 id="backup-files-title">数据迁移</h2>
        <small>导出全部学习数据和应用内设置，或从文件恢复。</small>
      </div>
      <div class="backup-file-actions">
        <VBtn
          color="primary"
          variant="tonal"
          rounded="lg"
          :loading="busy === 'export'"
          :disabled="!!busy"
          @click="exportData"
        >导出</VBtn>
        <VBtn
          color="primary"
          variant="tonal"
          rounded="lg"
          :loading="busy === 'preview'"
          :disabled="!!busy"
          @click="chooseBackup"
        >恢复</VBtn>
      </div>
    </div>
  </section>

  <VDialog :model-value="!!preview" persistent max-width="540" aria-labelledby="restore-title">
    <VCard v-if="preview" rounded="lg">
      <VCardTitle id="restore-title">
        {{ restored ? "数据已恢复" : "从备份文件恢复" }}
      </VCardTitle>
      <VCardText v-if="restored">
        学习数据和应用内设置已替换。重新打开 StudyFlow 后即可使用。
        <VAlert v-if="restoreError" type="error" variant="tonal">{{ restoreError }}</VAlert>
      </VCardText>
      <VCardText v-else class="restore-preview">
        <strong class="restore-filename">{{ preview.filename }}</strong>
        <span>
          {{ preview.subjects }} 个学习项目 · {{ preview.sessions }} 条学习记录
        </span>
        <span v-if="preview.exportedAt">
          导出时间：{{ new Date(preview.exportedAt).toLocaleString("zh-CN") }}
        </span>
        <span v-if="preview.needsUpgrade">
          {{ preview.legacy ? "已识别旧版数据库" : `数据库版本 ${preview.version}` }}，恢复前将升级副本到版本 {{ preview.targetVersion }}。
        </span>
        <span v-if="preview.timezoneNote">{{ preview.timezoneNote }}</span>
        <VAlert type="warning" variant="tonal" density="compact">
          恢复会替换本机的全部学习数据和应用内设置。成功后不保留原数据库。
        </VAlert>
        <VAlert v-if="restoreError" type="error" variant="tonal" density="compact">
          {{ restoreError }}
        </VAlert>
      </VCardText>
      <div class="restore-actions">
        <VBtn v-if="!restored" variant="text" :disabled="!!busy" @click="closePreview">
          {{ restoreError ? "关闭" : "取消" }}
        </VBtn>
        <VBtn
          v-if="!restored && !restoreError"
          color="error"
          variant="flat"
          :loading="busy === 'restore'"
          @click="restoreData"
        >替换数据</VBtn>
        <VBtn v-if="restored" color="primary" variant="flat" :loading="busy === 'restart'" @click="reopen">
          重新打开
        </VBtn>
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.backup-files {
  border-top: 1px solid var(--outline-card);
  padding-block: var(--space-5);
}

.backup-files h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 650;
}

.backup-file-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas: "heading ." "description actions";
  gap: var(--space-1) var(--space-4);
}

.backup-file-row .section-setting-copy {
  display: contents;
}

.backup-file-row h2 {
  grid-area: heading;
}

.backup-file-row small {
  grid-area: description;
}

.backup-file-actions,
.restore-actions {
  display: flex;
  gap: var(--space-2);
  margin-inline-start: auto;
}

.backup-file-actions {
  grid-area: actions;
  align-self: center;
  align-items: center;
  /* 按小字这一行居中，按钮高度不把标题和说明撑开。 */
  block-size: 0;
}

.restore-preview {
  display: grid;
  gap: var(--space-2);
}

.restore-filename {
  overflow-wrap: anywhere;
}

.restore-actions {
  justify-content: flex-end;
  padding: var(--space-2) var(--space-5) var(--space-4);
}
</style>
