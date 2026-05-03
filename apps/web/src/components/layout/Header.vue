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
  <!-- TODO(tw-arb): spacing px-[22px] py-[10px] px-[9px] py-[3px] px-[7px] p-[1px] gap-[8px] gap-[10px] | size w-[24px] h-[13px] w-[11px] h-[11px] top-[1px] left-[1px] left-[12px] | radius rounded-[3px|4px|6px|7px] | text text-[11px|11.5px|12px|12.5px] font-[inherit] | misc shadow-[0_1px_2px_rgba(0,0,0,0.2)] transition-[left] -->
  <header
    class="flex items-center justify-between px-[22px] py-[10px] border-b border-border bg-background text-[12px]"
  >
    <div class="flex items-center gap-[10px] text-muted-foreground">
      <template v-for="(b, i) in breadcrumbs" :key="i">
        <span v-if="i > 0" class="text-subtle">/</span>
        <RouterLink
          v-if="b.to && i < breadcrumbs.length - 1"
          :to="b.to"
          class="bg-transparent border-none cursor-pointer text-muted-foreground text-[12.5px] no-underline"
        >{{ b.label }}</RouterLink>
        <span
          v-else
          class="text-[12.5px]"
          :class="i === breadcrumbs.length - 1 ? 'text-foreground font-medium' : 'text-muted-foreground font-normal'"
        >{{ b.label }}</span>
      </template>
    </div>

    <div class="flex items-center gap-[8px]">
      <button
        :title="ui.privacy ? 'Показать суммы' : 'Скрыть суммы (privacy)'"
        @click="ui.togglePrivacy"
        class="inline-flex items-center cursor-pointer gap-[8px] px-[9px] py-[3px] border border-border rounded-[4px] bg-panel text-muted-foreground text-[11.5px] font-[inherit]"
      >
        <span :class="ui.privacy ? 'text-foreground' : 'text-muted-foreground'">Privacy</span>
        <span
          class="relative inline-block w-[24px] h-[13px] rounded-[7px] transition-colors duration-150"
          :class="ui.privacy ? 'bg-accent' : 'bg-subtle'"
        >
          <span
            class="absolute top-[1px] w-[11px] h-[11px] rounded-[6px] bg-white shadow-[0_1px_2px_rgba(0,0,0,0.2)] transition-[left] duration-150"
            :class="ui.privacy ? 'left-[12px]' : 'left-[1px]'"
          />
        </span>
      </button>

      <div class="flex border border-border rounded-[4px] p-[1px] bg-panel">
        <button
          v-for="c in ui.SUPPORTED_CURRENCIES"
          :key="c"
          @click="ui.displayCurrency = c"
          class="border-none px-[9px] py-[3px] rounded-[3px] text-[11.5px] cursor-pointer font-[inherit]"
          :class="ui.displayCurrency === c ? 'bg-soft text-foreground font-medium' : 'bg-transparent text-muted-foreground font-normal'"
        >{{ c }}</button>
      </div>

      <div
        class="flex cursor-pointer border border-border rounded-[4px] p-[1px] bg-panel"
        @click="ui.toggleTheme"
      >
        <span
          class="px-[7px] py-[3px] rounded-[3px] text-[11px]"
          :class="ui.theme !== 'dark' ? 'bg-soft text-foreground' : 'bg-transparent text-muted-foreground'"
        >☀</span>
        <span
          class="px-[7px] py-[3px] rounded-[3px] text-[11px]"
          :class="ui.theme === 'dark' ? 'bg-soft text-foreground' : 'bg-transparent text-muted-foreground'"
        >☾</span>
      </div>

      <button
        v-if="auth.user"
        @click="handleLogout"
        :title="`Выйти (${auth.user.email})`"
        class="bg-transparent border border-border text-muted-foreground px-[9px] py-[3px] rounded-[4px] text-[11.5px] cursor-pointer font-[inherit]"
      >Выйти</button>
    </div>
  </header>
</template>
