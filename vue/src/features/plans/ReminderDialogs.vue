<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { api, type DailyPlanHistory } from "../../api";
import { dateKey } from "../../shared/format";
import { useAppStore } from "../../stores/app";
import { usePlansStore } from "../../stores/plans";
import { useSessionsStore } from "../../stores/sessions";
const appStore = useAppStore();
const plansStore = usePlansStore();
const sessionsStore = useSessionsStore();
const {
  showTaskReminder,
  taskReminderEnded,
  showBreakReminder,
  showBreakExhausted,
  breakReminderSubject,
  todayPaused,
} = storeToRefs(sessionsStore);
const { dismissTaskReminder, finishBreakReminder } = sessionsStore;

// 提醒用的当前计划与下一次计划（未暂停时取当前计划；向后找最近一次开始）。
const reminderCurrentPlan = computed(() =>
  todayPaused.value ? undefined : plansStore.currentPlan,
);

const futurePlans = ref<Record<string, DailyPlanHistory>>({});
watch(showTaskReminder, async (open, _, onCleanup) => {
  if (!open) return;
  let cancelled = false;
  onCleanup(() => { cancelled = true; });
  futurePlans.value = {};
  const now = new Date(appStore.now);
  const days = Array.from({ length: 7 }, (_, index) =>
    dateKey(new Date(now.getFullYear(), now.getMonth(), now.getDate() + index + 1, 12)),
  );
  try {
    const result = await api.planDays(days);
    if (!cancelled) futurePlans.value = result;
  } catch (error) {
    if (!cancelled) appStore.setError(error);
  }
});

const nextReminderPlan = computed(() => {
  const current = new Date(appStore.now);
  // 向后最多找 7 天（含今天共 8 天），足以覆盖任意每周重复计划的下一次开始。
  for (let offset = 0; offset < 8; offset++) {
    if (offset === 0 && todayPaused.value) continue;
    const day = new Date(
      current.getFullYear(),
      current.getMonth(),
      current.getDate() + offset,
    );
    const weekday = day.getDay();
    const candidates = (offset === 0 ? plansStore.plans : futurePlans.value[dateKey(day)]?.plans ?? [])
      .filter(
        (p) => p.active && p.weekdays.split(",").map(Number).includes(weekday),
      )
      .sort((a, b) => a.startTime.localeCompare(b.startTime));
    for (const p of candidates) {
      const [hour, minute] = p.startTime.split(":").map(Number);
      const start = new Date(
        day.getFullYear(),
        day.getMonth(),
        day.getDate(),
        hour,
        minute,
      );
      if (start.getTime() > appStore.now) return { plan: p, start };
    }
  }
  return null;
});
</script>

<template>
  <div v-if="showBreakExhausted" class="modal-backdrop">
    <div class="modal plan-confirm">
      <h2>休息时间已用完</h2>
      <p>本次学习计划的休息额度已经用完，请继续学习或结束本次学习。</p>
      <button
        class="btn primary modal-submit"
        @click="showBreakExhausted = false"
      >
        知道了
      </button>
    </div>
  </div>
  <div v-if="showBreakReminder" class="modal-backdrop">
    <div class="modal task-reminder-modal">
      <div class="reminder-bell">☕</div>
      <h2>休息时间结束</h2>
      <p>{{ breakReminderSubject }} · 可以继续学习了</p>
      <div class="confirm-actions">
        <button class="btn secondary" @click="finishBreakReminder(false)">
          稍后继续</button
        ><button class="btn primary" @click="finishBreakReminder(true)">
          继续学习
        </button>
      </div>
    </div>
  </div>
  <VDialog
    :model-value="showTaskReminder"
    max-width="480"
    :z-index="2300"
    @update:model-value="!$event && dismissTaskReminder()"
    ><VCard
      v-if="showTaskReminder"
      class="plan-confirm-dialog plan-dialog-card"
      rounded="xl"
      ><VCardTitle class="plan-dialog-title">{{
        taskReminderEnded ? "学习时段已结束" : "学习计划提醒"
      }}</VCardTitle
      ><VCardText class="plan-confirm-content"
        ><p v-if="taskReminderEnded">
          「{{ taskReminderEnded.name }}」已结束。<span
            v-if="!taskReminderEnded.saved"
            >本次不足 10 分钟，未计入学习记录。</span
          >
        </p>
        <p v-if="reminderCurrentPlan">
          当前计划「{{ reminderCurrentPlan.subject }}」现在可以开始。
        </p>
        <p v-else-if="nextReminderPlan">
          当前没有正在进行的学习计划。下一项「{{
            nextReminderPlan.plan.subject
          }}」将于
          {{
            nextReminderPlan.start.toLocaleString("zh-CN", {
              month: "numeric",
              day: "numeric",
              hour: "2-digit",
              minute: "2-digit",
            })
          }}
          开始。
        </p>
        <p v-else>当前没有正在进行的学习计划。</p></VCardText
      >
      <div class="subject-form-actions plan-confirm-actions">
        <VBtn
          variant="tonal"
          color="primary"
          rounded="lg"
          @click="dismissTaskReminder"
          >知道了</VBtn
        >
      </div></VCard
    ></VDialog
  >
</template>

<style scoped>
.modal-submit {
  width: 100%;
  margin-top: 22px;
}
.modal-submit {
  margin-top: 24px;
}

.reminder-bell {
  width: 48px;
  height: 48px;
  margin: 0 auto 14px;
  display: grid;
  place-items: center;
  border-radius: 15px;
  background: #eef1ff;
  color: #2932e1;
  font-size: 24px;
}
.task-reminder-modal {
  text-align: center;
}
.task-reminder-modal h2 {
  font-size: 23px;
}
.task-reminder-modal > p {
  margin: 12px 0 18px;
  font-size: 16px;
  color: #565a65;
}
</style>
