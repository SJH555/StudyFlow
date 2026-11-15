import { ref } from "vue";

// 休息悬浮球拖拽：维护悬浮球位置，处理指针拖拽与窗口边界约束。
export function useRestOrb() {
  // 初始放在窗口右上区域；132/124 为球的尺寸加边距。
  const restOrbX = ref(Math.max(8, window.innerWidth - 132)),
    restOrbY = ref(220);

  let restDragOffsetX = 0,
    restDragOffsetY = 0;

  let restDragging = false;

  function clampRestOrb() {
    restOrbX.value = Math.max(
      8,
      Math.min(window.innerWidth - 124, restOrbX.value),
    );
    restOrbY.value = Math.max(
      8,
      Math.min(window.innerHeight - 124, restOrbY.value),
    );
  }

  function beginRestDrag(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    restDragging = true;
    restDragOffsetX = event.clientX - restOrbX.value;
    restDragOffsetY = event.clientY - restOrbY.value;
    document.body.classList.add("rest-orb-dragging");
    document.getSelection()?.removeAllRanges();
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  }

  function moveRestDrag(event: PointerEvent) {
    if (!restDragging) return;
    event.preventDefault();
    document.getSelection()?.removeAllRanges();
    restOrbX.value = event.clientX - restDragOffsetX;
    restOrbY.value = event.clientY - restDragOffsetY;
    clampRestOrb();
  }

  function endRestDrag(event: PointerEvent) {
    restDragging = false;
    document.body.classList.remove("rest-orb-dragging");
    const element = event.currentTarget as HTMLElement;
    if (element.hasPointerCapture(event.pointerId))
      element.releasePointerCapture(event.pointerId);
  }

  return {
    restOrbX,
    restOrbY,
    clampRestOrb,
    beginRestDrag,
    moveRestDrag,
    endRestDrag,
  };
}
