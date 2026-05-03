export { default as Table } from "./Table.vue";

import { defineComponent, h } from "vue";
import { cn } from "@/lib/utils";

export const TableHeader = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () => h("thead", { class: cn("[&_tr]:border-b", props.class) }, slots.default?.());
  },
});

export const TableBody = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h("tbody", { class: cn("[&_tr:last-child]:border-0", props.class) }, slots.default?.());
  },
});

export const TableRow = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h(
        "tr",
        {
          class: cn(
            "border-b transition-colors hover:bg-muted/50",
            props.class,
          ),
        },
        slots.default?.(),
      );
  },
});

export const TableHead = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h(
        "th",
        {
          class: cn(
            "h-10 px-3 text-left align-middle font-medium text-muted-foreground",
            props.class,
          ),
        },
        slots.default?.(),
      );
  },
});

export const TableCell = defineComponent({
  props: { class: { type: String, default: "" } },
  setup(props, { slots }) {
    return () =>
      h("td", { class: cn("p-3 align-middle", props.class) }, slots.default?.());
  },
});
