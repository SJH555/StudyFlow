import { ref } from "vue";
import { Window as appWindow } from "@wailsio/runtime";

// 窗口控制：负责自定义标题栏的最小化/最大化/关闭操作与最大化状态同步。
// 状态为模块级共享（App、生命周期与各弹窗读取同一份）。
const windowsTitlebar = ref(/Windows/i.test(navigator.userAgent)),
  windowsMaximised = ref(false);

export function useWindowFrame(setError: (error: unknown) => void) {
  async function syncWindowMaximised() {
    if (windowsTitlebar.value)
      try {
        windowsMaximised.value = await appWindow.IsMaximised();
      } catch {}
  }

  async function windowAction(action: "minimise" | "maximise" | "close") {
    try {
      if (action === "minimise") await appWindow.Minimise();
      else if (action === "maximise") {
        await appWindow.ToggleMaximise();
        await syncWindowMaximised();
      } else await appWindow.Close();
    } catch (e) {
      setError(e);
    }
  }

  return {
    windowsTitlebar,
    windowsMaximised,
    syncWindowMaximised,
    windowAction,
  };
}
