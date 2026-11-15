<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { clockLabel } from "../../shared/format";
import { useRestOrb } from "./useRestOrb";
import { useActiveLearning } from "../sessions/useActiveLearning";

// 休息悬浮球：组件内持有拖拽位置，窗口尺寸变化时自行约束边界。
const { restOrbX, restOrbY, clampRestOrb, beginRestDrag, moveRestDrag, endRestDrag } =
  useRestOrb();
const { activeBreakPlan, breakLeftSeconds } = useActiveLearning();

onMounted(() => window.addEventListener("resize", clampRestOrb));
onUnmounted(() => window.removeEventListener("resize", clampRestOrb));
</script>

<template>
  <div
    v-if="activeBreakPlan"
    class="rest-orb"
    :style="{ left: restOrbX + 'px', top: restOrbY + 'px' }"
    role="status"
    title="拖动调整位置"
    @pointerdown="beginRestDrag"
    @pointermove="moveRestDrag"
    @pointerup="endRestDrag"
    @pointercancel="endRestDrag"
    @dragstart.prevent
  >
    <span class="rest-orb-icon">☕</span
    ><b>{{ breakLeftSeconds === 0 ? "休息结束" : "休息中" }}</b
    ><strong>{{ clockLabel(breakLeftSeconds) }}</strong>
  </div>
</template>

<style scoped>
.rest-orb {
  position: fixed;
  z-index: 15;
  width: 116px;
  height: 116px;
  border-radius: 50%;
  border: 3px solid #fff;
  background: linear-gradient(145deg, #4c55f0, #2029c4);
  color: #fff;
  box-shadow:
    0 13px 32px #2932e15c,
    0 0 0 1px #2932e133;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  cursor: grab;
  touch-action: none;
  user-select: none;
  transition: box-shadow 0.2s ease;
}
.rest-orb:active {
  cursor: grabbing;
  box-shadow:
    0 17px 40px #2932e16e,
    0 0 0 4px #bfc9ff66;
}
.rest-orb-icon {
  font-size: 19px;
  line-height: 1;
}
.rest-orb b {
  font-size: 12px;
  font-weight: 650;
}
.rest-orb strong {
  font-size: 21px;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.3px;
  line-height: 1.15;
}
</style>
