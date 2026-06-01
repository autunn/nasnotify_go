import { injectHtml } from "@ugreen-nas/builder-core";
import { UgosViteBuilder } from "@ugreen-nas/builder-open/vite";
import legacy from "@vitejs/plugin-legacy";
import { defineConfig } from "vite";

const builder = new UgosViteBuilder({
  windowConfig: {
    width: 1440,
    height: 900,
    background: "#15181d",
    hideOnClose: true,
    frame: false,
    hideTitle: true,
    showIcon: false,
    color: "rgba(255, 255, 255, 0.96)",
    blurColor: "rgba(203, 213, 225, 0.78)",
    headConfig: {
      height: 56
    }
  }
});

builder.hooks.htmlInjection.tap((_current, isDev) =>
  injectHtml(isDev, {
    template: "full"
  })
);

export default defineConfig({
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: ["es2015", "chrome58", "safari11"]
  },
  plugins: [
    ...builder.pluginEntry(),
    legacy({
      modernPolyfills: true,
      renderLegacyChunks: true,
      targets: ["defaults", "chrome >= 58", "safari >= 11"]
    })
  ]
});
