<script setup lang="ts">
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { useUiStore } from "@/stores/ui";
import { useQueryClient } from "@tanstack/vue-query";
import { logout as apiLogout } from "@/api/generated";
import { Button } from "@/components/ui/button";

const auth = useAuthStore();
const ui = useUiStore();
const router = useRouter();
const queryClient = useQueryClient();

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
    class="border-b sticky top-0 z-30"
    style="background-color: hsl(var(--background))"
  >
    <div class="container mx-auto px-4 h-14 flex items-center gap-4">
      <RouterLink to="/" class="text-lg font-semibold">Omnifolio</RouterLink>
      <nav class="flex items-center gap-4 text-sm">
        <RouterLink to="/" class="hover:underline">Дашборд</RouterLink>
        <RouterLink to="/accounts" class="hover:underline">Аккаунты</RouterLink>
      </nav>
      <div class="ml-auto flex items-center gap-3">
        <select
          v-model="ui.displayCurrency"
          class="rounded border px-2 py-1 text-sm"
          style="background-color: hsl(var(--background))"
        >
          <option v-for="c in ui.SUPPORTED_CURRENCIES" :key="c" :value="c">
            {{ c }}
          </option>
        </select>
        <Button variant="ghost" size="sm" @click="ui.toggleTheme">
          {{ ui.theme === "dark" ? "☀️" : "🌙" }}
        </Button>
        <span v-if="auth.user" class="text-sm opacity-75">
          {{ auth.user.email }}
        </span>
        <Button variant="outline" size="sm" @click="handleLogout">Выйти</Button>
      </div>
    </div>
  </header>
</template>
