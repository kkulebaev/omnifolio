<script setup lang="ts">
import { watch } from "vue";
import { useRoute } from "vue-router";
import Header from "./Header.vue";
import Sidebar from "./Sidebar.vue";
import { ConfirmDialog } from "@/components/ui/confirm";
import { useUiStore } from "@/stores/ui";

const ui = useUiStore();
const route = useRoute();

watch(() => route.fullPath, () => ui.closeMobileSidebar());
</script>

<template>
  <div class="flex h-screen bg-background text-foreground">
    <Sidebar />
    <div
      v-if="ui.mobileSidebarOpen"
      class="md:hidden fixed inset-0 z-40 bg-black/50"
      @click="ui.closeMobileSidebar"
    />
    <main class="flex-1 flex flex-col overflow-hidden min-w-0">
      <Header />
      <div class="flex-1 overflow-auto">
        <slot />
      </div>
    </main>
    <ConfirmDialog />
  </div>
</template>
