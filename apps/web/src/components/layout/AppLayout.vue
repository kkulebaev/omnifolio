<script setup lang="ts">
import Header from "./Header.vue";
import Sidebar from "./Sidebar.vue";
</script>

<template>
  <!-- TODO(tw-arb): grid-cols-[188px_1fr] -->
  <div class="grid grid-cols-[188px_1fr] h-screen bg-background text-foreground">
    <Sidebar />
    <main class="flex flex-col overflow-hidden">
      <Header />
      <div class="flex-1 overflow-auto">
        <slot />
      </div>
    </main>
  </div>
</template>
