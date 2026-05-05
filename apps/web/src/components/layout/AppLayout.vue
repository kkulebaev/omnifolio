<script setup lang="ts">
import Header from "./Header.vue";
import Sidebar from "./Sidebar.vue";
import { ConfirmDialog } from "@/components/ui/confirm";
</script>

<template>
  <div class="flex h-screen bg-background text-foreground">
    <Sidebar />
    <main class="flex-1 flex flex-col overflow-hidden">
      <Header />
      <div class="flex-1 overflow-auto">
        <slot />
      </div>
    </main>
    <ConfirmDialog />
  </div>
</template>
