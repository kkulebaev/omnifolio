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
    segments.push({ label: "Dashboard" });
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
  <header
    class="flex items-center justify-between"
    style="
      padding: 10px 22px;
      border-bottom: 1px solid hsl(var(--border));
      background-color: hsl(var(--background));
      font-size: 12px;
    "
  >
    <div
      class="flex items-center"
      style="gap: 10px; color: hsl(var(--muted-foreground))"
    >
      <template v-for="(b, i) in breadcrumbs" :key="i">
        <span v-if="i > 0" style="color: hsl(var(--subtle))">/</span>
        <RouterLink
          v-if="b.to && i < breadcrumbs.length - 1"
          :to="b.to"
          style="
            background: transparent;
            border: none;
            cursor: pointer;
            color: hsl(var(--muted-foreground));
            font-size: 12.5px;
            text-decoration: none;
          "
        >{{ b.label }}</RouterLink>
        <span
          v-else
          :style="{
            color: i === breadcrumbs.length - 1 ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            fontWeight: i === breadcrumbs.length - 1 ? 500 : 400,
            fontSize: '12.5px',
          }"
        >{{ b.label }}</span>
      </template>
    </div>

    <div class="flex items-center" style="gap: 8px">
      <button
        :title="ui.privacy ? 'Показать суммы' : 'Скрыть суммы (privacy)'"
        @click="ui.togglePrivacy"
        class="inline-flex items-center cursor-pointer"
        :style="{
          gap: '8px',
          padding: '3px 9px',
          border: '1px solid hsl(var(--border))',
          borderRadius: '4px',
          background: 'hsl(var(--panel))',
          color: 'hsl(var(--muted-foreground))',
          fontSize: '11.5px',
          fontFamily: 'inherit',
        }"
      >
        <span
          :style="{ color: ui.privacy ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))' }"
        >Privacy</span>
        <span
          :style="{
            position: 'relative',
            display: 'inline-block',
            width: '24px',
            height: '13px',
            borderRadius: '7px',
            background: ui.privacy ? 'hsl(var(--accent))' : 'hsl(var(--subtle))',
            transition: 'background .15s',
          }"
        >
          <span
            :style="{
              position: 'absolute',
              top: '1px',
              left: ui.privacy ? '12px' : '1px',
              width: '11px',
              height: '11px',
              borderRadius: '6px',
              background: '#fff',
              boxShadow: '0 1px 2px rgba(0,0,0,0.2)',
              transition: 'left .15s',
            }"
          />
        </span>
      </button>

      <div
        class="flex"
        style="
          border: 1px solid hsl(var(--border));
          border-radius: 4px;
          padding: 1px;
          background: hsl(var(--panel));
        "
      >
        <button
          v-for="c in ui.SUPPORTED_CURRENCIES"
          :key="c"
          @click="ui.displayCurrency = c"
          :style="{
            background: ui.displayCurrency === c ? 'hsl(var(--soft))' : 'transparent',
            border: 'none',
            color: ui.displayCurrency === c ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            padding: '3px 9px',
            borderRadius: '3px',
            fontSize: '11.5px',
            fontWeight: ui.displayCurrency === c ? 500 : 400,
            cursor: 'pointer',
            fontFamily: 'inherit',
          }"
        >{{ c }}</button>
      </div>

      <div
        class="flex cursor-pointer"
        @click="ui.toggleTheme"
        style="
          border: 1px solid hsl(var(--border));
          border-radius: 4px;
          padding: 1px;
          background: hsl(var(--panel));
        "
      >
        <span
          :style="{
            padding: '3px 7px',
            borderRadius: '3px',
            background: ui.theme !== 'dark' ? 'hsl(var(--soft))' : 'transparent',
            color: ui.theme !== 'dark' ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            fontSize: '11px',
          }"
        >☀</span>
        <span
          :style="{
            padding: '3px 7px',
            borderRadius: '3px',
            background: ui.theme === 'dark' ? 'hsl(var(--soft))' : 'transparent',
            color: ui.theme === 'dark' ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            fontSize: '11px',
          }"
        >☾</span>
      </div>

      <button
        v-if="auth.user"
        @click="handleLogout"
        :title="`Выйти (${auth.user.email})`"
        style="
          background: transparent;
          border: 1px solid hsl(var(--border));
          color: hsl(var(--muted-foreground));
          padding: 3px 9px;
          border-radius: 4px;
          font-size: 11.5px;
          cursor: pointer;
          font-family: inherit;
        "
      >Выйти</button>
    </div>
  </header>
</template>
