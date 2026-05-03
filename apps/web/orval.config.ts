import { defineConfig } from "orval";

export default defineConfig({
  omnifolio: {
    input: {
      target: "../../api/openapi.yaml",
    },
    output: {
      target: "src/api/generated/index.ts",
      schemas: "src/api/generated/model",
      mode: "split",
      client: "vue-query",
      mock: false,
      override: {
        mutator: {
          path: "src/api/mutator.ts",
          name: "fetcher",
        },
      },
    },
  },
});
