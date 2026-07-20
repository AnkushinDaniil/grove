import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test/setup.ts"],
      css: false,
      // Tests run against the mock transport/API, never a real backend.
      env: { VITE_MOCK: "1" },
      coverage: {
        provider: "v8",
        reporter: ["text", "html"],
      },
    },
  }),
);
