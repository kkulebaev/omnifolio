<script setup lang="ts">
import Header from "./Header.vue";
</script>

<template>
  <div class="min-h-screen flex flex-col">
    <Header />
    <main class="container mx-auto px-4 py-6 flex-1">
      <slot />
    </main>
  </div>
</template>
