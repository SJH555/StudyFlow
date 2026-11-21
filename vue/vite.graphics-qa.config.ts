import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { homedir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

// 仅供隔离的图形资源对照，正常构建仍使用 vite.config.ts。
const mode = process.env.STUDYFLOW_GRAPHICS_QA;
if (!mode || !["shell", "none", "canvas", "svg"].includes(mode)) {
  throw new Error("必须显式指定 STUDYFLOW_GRAPHICS_QA=shell/none/canvas/svg");
}

export default defineConfig({
  root: fileURLToPath(new URL(".", import.meta.url)),
  base: "./",
  plugins: [
    {
      name: "studyflow-graphics-experiment",
      enforce: "pre",
      transform(source, id) {
        if (mode === "shell" && id.endsWith("/src/App.vue")) {
          if (!/<template>[\s\S]*?<\/template>/.test(source)) {
            throw new Error("空外壳实验未找到根模板，拒绝继续构建");
          }
          return source.replace(
            /<template>[\s\S]*?<\/template>/,
            '<template><main data-qa-shell="true" style="height:100vh;background:white"></main></template>',
          );
        }
        if (!id.endsWith("/src/features/stats/useCharts.ts")) return;
        if (mode === "svg") {
          for (const expected of [
            "CanvasRenderer",
            "echarts.init(chartEl.value)",
            "echarts.init(projectChartEl.value)",
          ]) {
            if (!source.includes(expected)) {
              throw new Error(`SVG 实验未找到 ${expected}，拒绝继续构建`);
            }
          }
          return source
            .replaceAll("CanvasRenderer", "SVGRenderer")
            .replaceAll(
              "echarts.init(chartEl.value)",
              'echarts.init(chartEl.value, undefined, { renderer: "svg" })',
            )
            .replaceAll(
              "echarts.init(projectChartEl.value)",
              'echarts.init(projectChartEl.value, undefined, { renderer: "svg" })',
            );
        }
        if (mode === "none") {
          for (const expected of [
            "function renderChart() {",
            "function renderProjectChart() {",
          ]) {
            if (!source.includes(expected)) {
              throw new Error(`无图表实验未找到 ${expected}，拒绝继续构建`);
            }
          }
          return source
            .replace(
              "function renderChart() {",
              "function renderChart() { return;",
            )
            .replace(
              "function renderProjectChart() {",
              "function renderProjectChart() { return;",
            );
        }
      },
    },
    vue(),
  ],
  build: {
    outDir: join(homedir(), ".cache", "studyflow-graphics-qa", mode, "dist"),
    emptyOutDir: true,
  },
});
