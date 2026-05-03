export { default as Card } from "./Card.vue";

import { defineComponent, h } from "vue";
import { cn } from "@/lib/utils";

export const CardHeader = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h("div", { class: cn("flex flex-col space-y-1.5 p-6", props.class) }, slots.default?.());
  },
});

export const CardTitle = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h(
        "h3",
        { class: cn("text-lg font-semibold leading-none tracking-tight", props.class) },
        slots.default?.(),
      );
  },
});

export const CardDescription = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h("p", { class: cn("text-sm text-muted-foreground", props.class) }, slots.default?.());
  },
});

export const CardContent = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () => h("div", { class: cn("p-6 pt-0", props.class) }, slots.default?.());
  },
});

export const CardFooter = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h("div", { class: cn("flex items-center p-6 pt-0", props.class) }, slots.default?.());
  },
});
