<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useUiStore } from "@/stores/ui";
import { useQueryClient } from "@tanstack/vue-query";
import { logout as apiLogout } from "@/api/generated";

const auth = useAuthStore();
const ui = useUiStore();
const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();

const breadcrumbs = computed<Array<{ label: string; to?: string }>>(() => {
  const segments: Array<{ label: string; to?: string }> = [
    { label: "Omnifolio", to: "/" },
  ];
  if (route.path === "/") {
    segments.push({ label: "Дэшборд" });
  } else if (route.path.startsWith("/accounts")) {
    segments.push({ label: "Аккаунты", to: "/accounts" });
    if (route.params.id) {
      segments.push({ label: String(route.params.id).slice(0, 8) });
    }
  } else if (route.path.startsWith("/instruments")) {
    segments.push({ label: "Инструменты" });
  } else if (route.path.startsWith("/settings")) {
    segments.push({ label: "Настройки" });
  }
  return segments;
});

async function handleLogout() {
  try {
    await apiLogout();
  } catch {
    /* ignore */
  }
  queryClient.clear();
  auth.clear();
  router.push({ name: "login" });
}
</script>

<template>
  <header
    class="flex items-center justify-between px-6 py-2.5 border-b border-border bg-background text-xs"
  >
    <div class="flex items-center gap-2.5 text-muted-foreground">
      <template v-for="(b, i) in breadcrumbs" :key="i">
        <span v-if="i > 0" class="text-subtle">/</span>
        <RouterLink
          v-if="b.to && i < breadcrumbs.length - 1"
          :to="b.to"
          class="bg-transparent border-none cursor-pointer text-muted-foreground text-xs no-underline"
        >{{ b.label }}</RouterLink>
        <span
          v-else
          class="text-xs"
          :class="i === breadcrumbs.length - 1 ? 'text-foreground font-medium' : 'text-muted-foreground font-normal'"
        >{{ b.label }}</span>
      </template>
    </div>

    <div class="flex items-center gap-2">
      <button
        :title="ui.privacy ? 'Показать суммы' : 'Скрыть суммы (privacy)'"
        @click="ui.togglePrivacy"
        class="inline-flex items-center cursor-pointer gap-2 px-2.5 py-1 border border-border rounded-sm bg-panel text-muted-foreground text-xs"
      >
        <span :class="ui.privacy ? 'text-foreground' : 'text-muted-foreground'">Privacy</span>
        <span
          class="relative inline-block w-6 h-3.5 rounded-full transition-colors duration-150"
          :class="ui.privacy ? 'bg-accent' : 'bg-subtle'"
        >
          <span
            class="absolute top-px w-3 h-3 rounded-full bg-white shadow-sm transition-all duration-150"
            :class="ui.privacy ? 'left-3' : 'left-px'"
          />
        </span>
      </button>

      <div class="flex border border-border rounded-sm p-px bg-panel">
        <button
          v-for="c in ui.SUPPORTED_CURRENCIES"
          :key="c"
          @click="ui.displayCurrency = c"
          class="border-none px-2.5 py-1 rounded-sm text-xs cursor-pointer"
          :class="ui.displayCurrency === c ? 'bg-soft text-foreground font-medium' : 'bg-transparent text-muted-foreground font-normal'"
        >{{ c }}</button>
      </div>

      <div
        class="flex cursor-pointer border border-border rounded-sm p-px bg-panel"
        @click="ui.toggleTheme"
      >
        <span
          class="px-2 py-1 rounded-sm text-xs"
          :class="ui.theme !== 'dark' ? 'bg-soft text-foreground' : 'bg-transparent text-muted-foreground'"
        >☀</span>
        <span
          class="px-2 py-1 rounded-sm text-xs"
          :class="ui.theme === 'dark' ? 'bg-soft text-foreground' : 'bg-transparent text-muted-foreground'"
        >☾</span>
      </div>

      <button
        v-if="auth.user"
        @click="handleLogout"
        :title="`Выйти (${auth.user.email})`"
        class="bg-transparent border border-border text-muted-foreground px-2.5 py-1 rounded-sm text-xs cursor-pointer"
      >Выйти</button>
    </div>
  </header>
</template>
