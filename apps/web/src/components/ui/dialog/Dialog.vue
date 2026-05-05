<script setup lang="ts">
const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ "update:open": [value: boolean] }>();

function close() {
  emit("update:open", false);
}
</script>

<template>
  <Teleport to="body">
    <div v-if="props.open" class="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div class="fixed inset-0 bg-black/50" @click="close" />
      <div
        class="relative z-10 w-full max-w-lg max-h-[calc(100vh-2rem)] overflow-y-auto rounded-lg border border-border bg-background p-4 sm:p-6 text-foreground shadow-lg"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>
