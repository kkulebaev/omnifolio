<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useUiStore } from "@/stores/ui";
import { useListAccounts, useGetPortfolio } from "@/api/generated";
import type { Account } from "@/api/generated/model/account";
import {
  LayoutDashboard,
  Wallet,
  Banknote,
  CandlestickChart,
  Building2,
  Settings,
} from "lucide-vue-next";

const route = useRoute();
const auth = useAuthStore();
const ui = useUiStore();

const navItems = [
  { id: "dash", label: "Дэшборд", icon: LayoutDashboard, to: "/", enabled: true },
  { id: "acc", label: "Аккаунты", icon: Wallet, to: "/accounts", enabled: true },
  { id: "dep", label: "Пополнения", icon: Banknote, to: "/deposits", enabled: true },
  { id: "ins", label: "Инструменты", icon: CandlestickChart, to: "/instruments", enabled: true },
  { id: "assets", label: "Имущество", icon: Building2, to: "/assets", enabled: true },
  { id: "set", label: "Настройки", icon: Settings, to: "/settings", enabled: true },
];

function isActive(to: string): boolean {
  if (to === "/") return route.path === "/";
  return route.path === to || route.path.startsWith(to + "/");
}

const accountsQuery = useListAccounts();
const portfolioQuery = useGetPortfolio(
  computed(() => ({ currency: ui.displayCurrency })),
  {
    query: {
      queryKey: computed(() => ["portfolio", ui.displayCurrency] as const),
    },
  },
);

const grandTotal = computed(
  () => Number(portfolioQuery.data.value?.summary.grandTotal ?? 0) || 0,
);
const byAccount = computed(
  () => portfolioQuery.data.value?.summary.byAccount ?? {},
);

const accountsWithShare = computed(() => {
  const list = accountsQuery.data.value?.items ?? [];
  const total = grandTotal.value || 1;
  return list
    .map((a: Account) => {
      const value = Number(byAccount.value[a.id] ?? 0) || 0;
      return {
        id: a.id,
        shortName: a.name.split(" · ")[0] || a.name,
        share: value / total,
      };
    })
    .sort((a, b) => b.share - a.share);
});

const userInitials = computed(() => {
  const email = auth.user?.email ?? "";
  return email.slice(0, 2).toUpperCase();
});
</script>

<template>
  <aside
    class="flex flex-col overflow-y-auto border-r border-border bg-background px-2.5 py-3.5 w-56 md:w-48 fixed md:static inset-y-0 left-0 z-50 transition-transform duration-200 md:translate-x-0"
    :class="ui.mobileSidebarOpen ? 'translate-x-0' : '-translate-x-full'"
  >
    <div class="flex items-center gap-2 pt-1 px-2 pb-4">
      <div
        class="grid place-items-center text-white font-bold w-5 h-5 rounded-sm bg-accent text-xs"
      >
        O
      </div>
      <span class="font-semibold text-sm tracking-tight">
        Omnifolio
      </span>
    </div>

    <nav class="flex flex-col gap-1">
      <RouterLink
        v-for="n in navItems.filter((i) => i.enabled)"
        :key="n.id"
        :to="n.to"
        custom
        v-slot="{ navigate }"
      >
        <button
          @click="navigate"
          class="flex items-center cursor-pointer text-left border-none px-2 py-1.5 rounded-md text-xs leading-none gap-2.5"
          :class="isActive(n.to) ? 'bg-soft text-foreground font-medium' : 'bg-transparent text-muted-foreground font-normal'"
        >
          <component
            :is="n.icon"
            :size="16"
            :stroke-width="1.75"
            class="shrink-0"
            :class="isActive(n.to) ? 'text-accent' : 'text-subtle'"
          />
          {{ n.label }}
        </button>
      </RouterLink>
      <button
        v-for="n in navItems.filter((i) => !i.enabled)"
        :key="n.id"
        disabled
        class="flex items-center text-left bg-transparent border-none text-subtle px-2 py-1.5 rounded-md text-xs leading-none gap-2.5 cursor-not-allowed"
      >
        <component :is="n.icon" :size="16" :stroke-width="1.75" class="shrink-0" />
        {{ n.label }}
      </button>
    </nav>

    <div class="mt-4">
      <div
        class="flex justify-between uppercase text-xs text-muted-foreground tracking-wider px-2 py-1.5"
      >
        <span>Аккаунты</span>
        <span class="num">{{ accountsWithShare.length }}</span>
      </div>
      <RouterLink
        v-for="a in accountsWithShare"
        :key="a.id"
        :to="`/accounts/${a.id}`"
        custom
        v-slot="{ navigate }"
      >
        <button
          @click="navigate"
          class="w-full text-left flex flex-col cursor-pointer bg-transparent border-none text-inherit px-2 py-1.5 gap-1 rounded-sm"
        >
          <div class="flex justify-between text-xs">
            <span class="overflow-hidden text-ellipsis whitespace-nowrap">
              {{ a.shortName }}
            </span>
            <span class="num text-muted-foreground text-xs">
              {{ (a.share * 100).toFixed(0) }}%
            </span>
          </div>
          <div class="h-0.5 bg-soft rounded-xs overflow-hidden">
            <div
              class="h-full bg-accent opacity-70"
              :style="{ width: `${a.share * 100}%` }"
            />
          </div>
        </button>
      </RouterLink>
    </div>

    <div class="flex items-center mt-auto p-2 gap-2 border-t border-border">
      <div
        class="grid place-items-center font-semibold w-6 h-6 rounded-xl bg-soft border border-border text-xs"
      >
        {{ userInitials }}
      </div>
      <div class="min-w-0 text-xs">
        <div class="font-medium">{{ auth.user?.email?.split("@")[0] ?? "—" }}</div>
        <div class="text-muted-foreground text-xs overflow-hidden text-ellipsis whitespace-nowrap">
          {{ auth.user?.email ?? "" }}
        </div>
      </div>
    </div>
  </aside>
</template>
