import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { api, type Plan } from "../api";
import { useAppStore } from "./app";

// 学习计划：计划数据的所有者，提供今日计划与当前进行中计划的派生。
export const usePlansStore = defineStore("plans", () => {
  const appStore = useAppStore();

  const plans = ref<Plan[]>([]);

  // 今日计划（按开始时间排序）。
  const todayPlans = computed(() => {
    // 用正午时间解析日期字符串，避免时区把日期偏移到前一天/后一天。
    const day = new Date(appStore.todayKey + "T12:00:00").getDay();
    return plans.value
      .filter(
        (p) => p.active && p.weekdays.split(",").map(Number).includes(day),
      )
      .sort((a, b) => a.startTime.localeCompare(b.startTime));
  });

  // 所选日期当天的计划列表（今日页与统计页共用）。
  const currentPlans = computed(() => {
    const wd = new Date(appStore.selectedDate + "T12:00:00").getDay();
    return plans.value
      .filter((p) => p.active && p.weekdays.split(",").map(Number).includes(wd))
      .sort((a, b) => a.startTime.localeCompare(b.startTime));
  });

  // 当前时刻正在进行的计划。
  const currentPlan = computed(() => {
    const current = new Date(appStore.now);
    // 同时检查今天与昨天，覆盖跨午夜仍在进行的计划。
    for (const offset of [0, -1]) {
      const day = new Date(
        current.getFullYear(),
        current.getMonth(),
        current.getDate() + offset,
      );
      for (const p of plans.value) {
        if (
          !p.active ||
          !p.weekdays.split(",").map(Number).includes(day.getDay())
        )
          continue;
        const [hour, minute] = p.startTime.split(":").map(Number);
        const start = new Date(
          day.getFullYear(),
          day.getMonth(),
          day.getDate(),
          hour,
          minute,
        );
        if (
          appStore.now >= start.getTime() &&
          appStore.now < start.getTime() + p.duration * 60_000
        )
          return p;
      }
    }
    return undefined;
  });

  async function fetch() {
    plans.value = await api.plans();
  }

  // 判断计划是否已到结束时间（区分进行中与已结束）。
  function hasPlanEnded(p: Plan) {
    return (
      appStore.now >=
      new Date(`${appStore.todayKey}T${p.startTime}:00`).getTime() +
        p.duration * 60_000
    );
  }

  // 接收外部拉取的数据（联合刷新时由 subjects store 编排写入）。
  function setPlans(list: Plan[]) {
    plans.value = list;
  }

  return { plans, todayPlans, currentPlans, currentPlan, fetch, hasPlanEnded, setPlans };
});
