<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useSettingsStore } from "../../stores/settings";
const settingsStore = useSettingsStore();
const { resetTarget, pendingDataDirectory } = storeToRefs(settingsStore);
const { confirmResetTable, saveDataDirectory } = settingsStore;
</script>

<template>
  <div v-if="pendingDataDirectory" class="modal-backdrop">
    <div class="modal plan-confirm">
      <h2>迁移学习数据？</h2>
      <p>
        将数据库和备份迁移到 {{ pendingDataDirectory }}。确认后立即执行迁移。
      </p>
      <div class="confirm-actions">
        <VBtn variant="text" rounded="lg" @click="pendingDataDirectory = ''"
          >取消</VBtn
        ><VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          prepend-icon="mdi-folder-move-outline"
          @click="saveDataDirectory"
          >确认迁移</VBtn
        >
      </div>
    </div>
  </div>
  <div v-if="resetTarget" class="modal-backdrop">
    <div class="modal delete-confirm">
      <h2>永久删除{{ resetTarget.label }}？</h2>
      <p>
        将清空 {{ resetTarget.label }}（{{ resetTarget.name }}）中的
        {{ resetTarget.count }} 条数据，此操作无法撤销。
      </p>
      <div class="confirm-actions">
        <button class="btn secondary" @click="resetTarget = null">取消</button
        ><button class="btn delete-confirm-button" @click="confirmResetTable">
          确认永久删除
        </button>
      </div>
    </div>
  </div>
</template>
