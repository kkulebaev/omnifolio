<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useUiStore } from "@/stores/ui";
import { useListAccounts, useGetPortfolio } from "@/api/generated";
import type { Account } from "@/api/generated/model/account";

const route = useRoute();
const auth = useAuthStore();
const ui = useUiStore();

const navItems = [
  { id: "dash", label: "Dashboard", icon: "◐", to: "/", enabled: true },
  { id: "acc", label: "Аккаунты", icon: "▤", to: "/accounts", enabled: true },
  { id: "port", label: "Портфели", icon: "◇", to: "/portfolios", enabled: false },
  { id: "ins", label: "Инструменты", icon: "⌗", to: "/instruments", enabled: false },
  { id: "set", label: "Настройки", icon: "⚙", to: "/settings", enabled: false },
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
  return list.map((a: Account) => {
    const value = Number(byAccount.value[a.id] ?? 0) || 0;
    return {
      id: a.id,
      shortName: a.name.split(" · ")[0] || a.name,
      share: value / total,
    };
  });
});

const userInitials = computed(() => {
  const email = auth.user?.email ?? "";
  return email.slice(0, 2).toUpperCase();
});
</script>

<template>
  <!-- TODO(tw-arb): spacing px-[10px] py-[14px] py-[5px] py-[6px] p-[8px] pt-[4px] pb-[16px] mt-[18px] gap-[3px] gap-[8px] gap-[9px] | size w-[188px] w-[18px] w-[14px] w-[24px] h-[18px] h-[24px] h-[2px] | radius rounded-[1px|4px|5px|12px] | text text-[10px|11px|11.5px|12px|12.5px|13px] font-[inherit] tracking-[-0.01em] tracking-[0.07em] -->
  <aside
    class="flex flex-col overflow-hidden border-r border-border bg-background w-[188px] px-[10px] py-[14px]"
  >
    <div class="flex items-center gap-2 pt-[4px] px-[8px] pb-[16px]">
      <div
        class="grid place-items-center text-white font-bold w-[18px] h-[18px] rounded-[4px] bg-accent text-[10px]"
      >
        O
      </div>
      <span class="font-semibold text-[13px] tracking-[-0.01em]">
        Omnifolio
      </span>
    </div>

    <nav class="flex flex-col gap-px">
      <RouterLink
        v-for="n in navItems.filter((i) => i.enabled)"
        :key="n.id"
        :to="n.to"
        custom
        v-slot="{ navigate }"
      >
        <button
          @click="navigate"
          class="flex items-center cursor-pointer text-left border-none px-[8px] py-[6px] rounded-[5px] text-[12.5px] gap-[9px] font-[inherit]"
          :class="isActive(n.to) ? 'bg-soft text-foreground font-medium' : 'bg-transparent text-muted-foreground font-normal'"
        >
          <span
            class="w-[14px] text-[11px]"
            :class="isActive(n.to) ? 'text-accent' : 'text-subtle'"
          >{{ n.icon }}</span>
          {{ n.label }}
        </button>
      </RouterLink>
      <button
        v-for="n in navItems.filter((i) => !i.enabled)"
        :key="n.id"
        disabled
        class="flex items-center text-left bg-transparent border-none text-subtle px-[8px] py-[6px] rounded-[5px] text-[12.5px] gap-[9px] font-[inherit] cursor-not-allowed"
      >
        <span class="w-[14px] text-[11px]">{{ n.icon }}</span>
        {{ n.label }}
      </button>
    </nav>

    <div class="mt-[18px]">
      <div
        class="flex justify-between uppercase text-[10px] text-muted-foreground tracking-[0.07em] px-[8px] py-[6px]"
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
          class="w-full text-left flex flex-col cursor-pointer bg-transparent border-none text-inherit px-[8px] py-[5px] gap-[3px] font-[inherit] rounded-[4px]"
        >
          <div class="flex justify-between text-[12px]">
            <span class="overflow-hidden text-ellipsis whitespace-nowrap">
              {{ a.shortName }}
            </span>
            <span class="num text-muted-foreground text-[11px]">
              {{ (a.share * 100).toFixed(0) }}%
            </span>
          </div>
          <div class="h-[2px] bg-soft rounded-[1px] overflow-hidden">
            <div
              class="h-full bg-accent opacity-70"
              :style="{ width: `${a.share * 100}%` }"
            />
          </div>
        </button>
      </RouterLink>
    </div>

    <div class="flex items-center mt-auto p-[8px] gap-[8px] border-t border-border">
      <div
        class="grid place-items-center font-semibold w-[24px] h-[24px] rounded-[12px] bg-soft border border-border text-[10px]"
      >
        {{ userInitials }}
      </div>
      <div class="min-w-0 text-[11.5px]">
        <div class="font-medium">{{ auth.user?.email?.split("@")[0] ?? "—" }}</div>
        <div class="text-muted-foreground text-[10px] overflow-hidden text-ellipsis whitespace-nowrap">
          {{ auth.user?.email ?? "" }}
        </div>
      </div>
    </div>
  </aside>
</template>
