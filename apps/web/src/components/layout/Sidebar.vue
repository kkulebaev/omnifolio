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
  <aside
    class="flex flex-col overflow-hidden border-r"
    style="width: 188px; background-color: hsl(var(--background)); border-color: hsl(var(--border)); padding: 14px 10px;"
  >
    <div class="flex items-center gap-2" style="padding: 4px 8px 16px">
      <div
        class="grid place-items-center text-white font-bold"
        style="
          width: 18px; height: 18px; border-radius: 4px;
          background-color: hsl(var(--accent)); font-size: 10px;
        "
      >
        O
      </div>
      <span class="font-semibold" style="font-size: 13px; letter-spacing: -0.01em">
        Omnifolio
      </span>
    </div>

    <nav class="flex flex-col" style="gap: 1px">
      <RouterLink
        v-for="n in navItems.filter((i) => i.enabled)"
        :key="n.id"
        :to="n.to"
        custom
        v-slot="{ navigate }"
      >
        <button
          @click="navigate"
          class="flex items-center cursor-pointer text-left"
          :style="{
            background: isActive(n.to) ? 'hsl(var(--soft))' : 'transparent',
            border: 'none',
            color: isActive(n.to) ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            padding: '6px 8px',
            borderRadius: '5px',
            fontSize: '12.5px',
            fontWeight: isActive(n.to) ? 500 : 400,
            gap: '9px',
            fontFamily: 'inherit',
          }"
        >
          <span
            :style="{
              width: '14px',
              color: isActive(n.to) ? 'hsl(var(--accent))' : 'hsl(var(--subtle))',
              fontSize: '11px',
            }"
          >{{ n.icon }}</span>
          {{ n.label }}
        </button>
      </RouterLink>
      <button
        v-for="n in navItems.filter((i) => !i.enabled)"
        :key="n.id"
        disabled
        class="flex items-center text-left"
        style="
          background: transparent;
          border: none;
          color: hsl(var(--subtle));
          padding: 6px 8px;
          border-radius: 5px;
          font-size: 12.5px;
          gap: 9px;
          font-family: inherit;
          cursor: not-allowed;
        "
      >
        <span style="width: 14px; font-size: 11px">{{ n.icon }}</span>
        {{ n.label }}
      </button>
    </nav>

    <div style="margin-top: 18px">
      <div
        class="flex justify-between uppercase"
        style="
          font-size: 10px;
          color: hsl(var(--muted-foreground));
          letter-spacing: 0.07em;
          padding: 6px 8px;
        "
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
          class="w-full text-left flex flex-col cursor-pointer"
          style="
            background: transparent;
            border: none;
            color: inherit;
            padding: 5px 8px;
            gap: 3px;
            font-family: inherit;
            border-radius: 4px;
          "
        >
          <div class="flex justify-between" style="font-size: 12px">
            <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
              {{ a.shortName }}
            </span>
            <span
              class="num"
              style="color: hsl(var(--muted-foreground)); font-size: 11px"
            >
              {{ (a.share * 100).toFixed(0) }}%
            </span>
          </div>
          <div
            style="
              height: 2px;
              background: hsl(var(--soft));
              border-radius: 1px;
              overflow: hidden;
            "
          >
            <div
              :style="{
                width: `${a.share * 100}%`,
                height: '100%',
                background: 'hsl(var(--accent))',
                opacity: 0.7,
              }"
            />
          </div>
        </button>
      </RouterLink>
    </div>

    <div
      class="flex items-center"
      style="
        margin-top: auto;
        padding: 8px;
        gap: 8px;
        border-top: 1px solid hsl(var(--border));
      "
    >
      <div
        class="grid place-items-center font-semibold"
        style="
          width: 24px; height: 24px; border-radius: 12px;
          background: hsl(var(--soft));
          border: 1px solid hsl(var(--border));
          font-size: 10px;
        "
      >
        {{ userInitials }}
      </div>
      <div style="min-width: 0; font-size: 11.5px">
        <div style="font-weight: 500">{{ auth.user?.email?.split("@")[0] ?? "—" }}</div>
        <div
          style="
            color: hsl(var(--muted-foreground));
            font-size: 10px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
          "
        >
          {{ auth.user?.email ?? "" }}
        </div>
      </div>
    </div>
  </aside>
</template>
