<script setup lang="ts">
import Header from "./Header.vue";
import Sidebar from "./Sidebar.vue";
</script>

<template>
  <div
    class="grid"
    style="
      grid-template-columns: 188px 1fr;
      height: 100vh;
      background-color: hsl(var(--background));
      color: hsl(var(--foreground));
    "
  >
    <Sidebar />
    <main class="flex flex-col overflow-hidden">
      <Header />
      <div class="flex-1 overflow-auto">
        <slot />
      </div>
    </main>
  </div>
</template>
