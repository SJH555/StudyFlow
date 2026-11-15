<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useAppStore } from "../../stores/app";
import { useSessionsStore } from "../../stores/sessions";
import { useActiveLearning } from "./useActiveLearning";
import { clockLabel, fmt, time } from "../../shared/format";
const { focusMode } = storeToRefs(useAppStore());
const { exitFocus } = useAppStore();
const sessionsStore = useSessionsStore();
const {
  showFinishConfirm,
  deleteSessionTarget,
  pendingStartPlan,
  active,
} = storeToRefs(sessionsStore);
const { sessionSeconds, startPlanNow, stop, deleteSession } = sessionsStore;
const { countdownLeft, countdownPercent } = useActiveLearning();
</script>

<template>
  <div v-if="pendingStartPlan" class="modal-backdrop">
    <div class="modal plan-confirm">
      <h2>跳过前面的计划？</h2>
      <p>
        前面有计划时段尚未完成。仍要先学习“{{ pendingStartPlan.subject }}”吗？
      </p>
      <div class="confirm-actions">
        <button class="btn secondary" @click="pendingStartPlan = null">
          取消</button
        ><button
          class="btn primary"
          @click="pendingStartPlan && startPlanNow(pendingStartPlan)"
        >
          确认开始
        </button>
      </div>
    </div>
  </div>
  <div v-if="showFinishConfirm" class="modal-backdrop">
    <div class="modal finish-confirm">
      <h2>结束本次学习？</h2>
      <p v-if="active && sessionSeconds(active) < 600">
        目前学习了 {{ clockLabel(sessionSeconds(active)) }}。不足 10
        分钟，结束后本次不会计入学习记录。
      </p>
      <p v-else>
        结束后会保存本次学习记录，时长为
        {{ active ? fmt(sessionSeconds(active)) : "" }}。
      </p>
      <div class="confirm-actions">
        <button class="btn secondary" @click="showFinishConfirm = false">
          继续学习</button
        ><button class="btn delete-confirm-button" @click="stop">
          确认结束
        </button>
      </div>
    </div>
  </div>
  <div v-if="focusMode" class="focus-overlay">
    <div class="focus-content">
      <div
        class="focus-ring"
        :style="{
          background: `conic-gradient(#94a7ff ${countdownPercent}%, #ffffff24 ${countdownPercent}%)`,
        }"
      >
        <div class="focus-ring-inner">
          <small>剩余时间</small
          ><strong>{{ clockLabel(countdownLeft) }}</strong>
        </div>
      </div>
      <button @click="exitFocus">退出倒计时</button>
    </div>
  </div>
  <VDialog
    :model-value="!!deleteSessionTarget"
    max-width="460"
    @update:model-value="!$event && (deleteSessionTarget = null)"
  >
    <VCard
      v-if="deleteSessionTarget"
      class="plan-confirm-dialog plan-dialog-card"
      rounded="xl"
    >
      <VCardTitle class="plan-dialog-title">删除学习记录？</VCardTitle>
      <VCardText class="plan-confirm-content"
        ><p>
          {{ deleteSessionTarget.subject }} ·
          {{ time(deleteSessionTarget.startAt) }} 开始
        </p></VCardText
      >
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn variant="text" rounded="lg" @click="deleteSessionTarget = null"
          >取消</VBtn
        ><VBtn variant="tonal" color="error" rounded="lg" @click="deleteSession"
          >确认删除</VBtn
        >
      </div>
    </VCard>
  </VDialog>
</template>

<style scoped>
.finish-confirm > p {
  font-size: 15px;
  line-height: 1.6;
  margin: 14px 0 20px;
  color: #5c6574;
}
.focus-overlay {
  position: fixed;
  inset: 0;
  z-index: 60;
  background: #111a33;
  color: #fff;
  display: grid;
  place-items: center;
}
.focus-content {
  display: grid;
  justify-items: center;
  gap: 28px;
}
.focus-content > span {
  font-size: 22px;
  font-weight: 650;
}
.focus-ring {
  width: min(62vw, 440px);
  height: min(62vw, 440px);
  border-radius: 50%;
  display: grid;
  place-items: center;
  box-shadow: 0 0 70px #6274dd30;
}
.focus-ring-inner {
  width: 90%;
  height: 90%;
  background: #111a33;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.focus-ring-inner small {
  font-size: 17px;
  color: #a9b6de;
}
.focus-ring-inner strong {
  font-size: clamp(58px, 8vw, 90px);
  font-variant-numeric: tabular-nums;
  line-height: 1;
}
.focus-content > button {
  border: 1px solid #ffffff55;
  background: #ffffff12;
  color: #fff;
  border-radius: 9px;
  padding: 11px 22px;
  font-size: 15px;
  cursor: pointer;
}
</style>
