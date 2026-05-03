<script setup lang="ts">
const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ "update:open": [value: boolean] }>();

function close() {
  emit("update:open", false);
}
</script>

<template>
  <Teleport to="body">
    <div v-if="props.open" class="fixed inset-0 z-50 flex items-center justify-center" role="dialog" aria-modal="true">
      <div class="fixed inset-0 bg-black/50" @click="close" />
      <div
        class="relative z-10 w-full max-w-lg rounded-lg border bg-background p-6 shadow-lg"
        style="background-color: hsl(var(--background)); color: hsl(var(--foreground))"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>
