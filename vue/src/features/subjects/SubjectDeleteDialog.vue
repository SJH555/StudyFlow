<script setup lang="ts">
import { useSubjectDraft } from "./useSubjectDraft";
import { useSubjectsStore } from "../../stores/subjects";
const { deleteSubjectTarget } = useSubjectDraft();
const { deleteSubject } = useSubjectsStore();
</script>

<template>
  <div v-if="deleteSubjectTarget" class="modal-backdrop">
    <div class="modal delete-confirm">
      <button class="modal-close" @click="deleteSubjectTarget = null">×</button>
      <h2>删除“{{ deleteSubjectTarget.name }}”？</h2>
      <div class="confirm-actions">
        <button class="btn secondary" @click="deleteSubjectTarget = null">
          取消</button
        ><button class="btn delete-confirm-button" @click="deleteSubject">
          确认删除
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-close {
  position: absolute;
  right: 17px;
  top: 13px;
  border: 0;
  background: none;
  font-size: 21px;
  color: #aaa89f;
}
.modal-close {
  font-size: 24px;
}
</style>
