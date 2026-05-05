<script setup lang="ts">
import {
  CheckboxRoot,
  CheckboxIndicator,
  type CheckboxRootProps,
} from "radix-vue";
import { Check } from "lucide-vue-next";
import { cn } from "@/lib/utils";

interface Props extends CheckboxRootProps {
  modelValue?: boolean;
  class?: string;
}

const props = defineProps<Props>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();
</script>

<template>
  <CheckboxRoot
    :id="props.id"
    :checked="props.modelValue ?? props.checked"
    :disabled="props.disabled"
    :required="props.required"
    :name="props.name"
    :value="props.value"
    :class="
      cn(
        'peer h-4 w-4 shrink-0 rounded-sm border border-input bg-background shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground',
        props.class,
      )
    "
    @update:checked="emit('update:modelValue', $event)"
  >
    <CheckboxIndicator class="flex h-full w-full items-center justify-center text-current">
      <Check class="h-3 w-3" :stroke-width="3" />
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
