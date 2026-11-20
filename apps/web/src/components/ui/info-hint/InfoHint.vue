<script setup lang="ts">
import {
  PopoverRoot,
  PopoverTrigger,
  PopoverPortal,
  PopoverContent,
} from "reka-ui";
import { Info } from "@lucide/vue";

defineProps<{ label: string }>();
</script>

<template>
  <!-- Popover rather than tooltip: hover-only hints are unreachable on touch screens. -->
  <PopoverRoot>
    <PopoverTrigger
      :aria-label="label"
      class="inline-flex items-center text-muted-foreground hover:text-foreground cursor-pointer bg-transparent border-none p-0 align-middle"
    >
      <Info :size="12" :stroke-width="1.75" />
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        side="top"
        :side-offset="6"
        :collision-padding="12"
        class="z-50 max-w-72 rounded-lg border border-border bg-panel px-3 py-2.5 shadow-md text-xs leading-relaxed normal-case tracking-normal font-normal text-foreground space-y-1.5"
      >
        <slot />
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>
