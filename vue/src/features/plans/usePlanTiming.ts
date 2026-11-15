import { computed, type Ref } from "vue";

// 计划时间联动：开始或结束时间决定时长，时长变化只推算结束时间。
export function usePlanTiming(
  hour: Ref<string>,
  minute: Ref<string>,
  duration: Ref<string>,
) {
  const minutesPerDay = 1440;
  const startMinutes = computed(
    () => Number(hour.value) * 60 + Number(minute.value),
  );
  const endMinutes = computed(() => {
    const value = Number(duration.value);
    return startMinutes.value + (Number.isInteger(value) ? value : 0);
  });
  const endClock = computed(
    () => (endMinutes.value % minutesPerDay + minutesPerDay) % minutesPerDay,
  );
  const timeError = computed(() => {
    if (!duration.value.trim()) return "";
    const value = Number(duration.value);
    if (!Number.isInteger(value)) return "";
    if (value <= 0) return "结束时间必须晚于开始时间";
    if (endMinutes.value >= minutesPerDay) {
      return "计划需在当天结束，请调整时间或缩短时长";
    }
    return "";
  });

  function updateDuration(end: number) {
    // 保留无效的时间差供表单提示，不能把早于开始的结束时刻解释为次日。
    duration.value = String(end - startMinutes.value);
  }

  const startHour = computed({
    get: () => hour.value,
    set: (value: string) => {
      const previousEnd = endClock.value;
      hour.value = value;
      updateDuration(previousEnd);
    },
  });
  const startMinute = computed({
    get: () => minute.value,
    set: (value: string) => {
      const previousEnd = endClock.value;
      minute.value = value;
      updateDuration(previousEnd);
    },
  });
  const endHour = computed({
    get: () => String(Math.floor(endClock.value / 60)).padStart(2, "0"),
    set: (value: string) => {
      updateDuration(Number(value) * 60 + endClock.value % 60);
    },
  });
  const endMinute = computed({
    get: () => String(endClock.value % 60).padStart(2, "0"),
    set: (value: string) => {
      updateDuration(Math.floor(endClock.value / 60) * 60 + Number(value));
    },
  });

  return { startHour, startMinute, endHour, endMinute, timeError };
}
