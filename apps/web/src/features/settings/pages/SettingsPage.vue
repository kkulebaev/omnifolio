<script setup lang="ts">
import { useRouter } from "vue-router";
import { useQueryClient } from "@tanstack/vue-query";
import { useAuthStore } from "@/stores/auth";
import { useUiStore } from "@/stores/ui";
import { logout as apiLogout } from "@/api/generated";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/formatters";
import { confirm } from "@/lib/confirm";

const auth = useAuthStore();
const ui = useUiStore();
const router = useRouter();
const queryClient = useQueryClient();

async function handleLogout() {
  const ok = await confirm({
    title: "Выйти из аккаунта?",
    confirmText: "Выйти",
  });
  if (!ok) return;
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
  <div class="space-y-6 p-6 max-w-3xl">
    <div>
      <h1 class="text-2xl font-semibold">Настройки</h1>
      <p class="text-sm text-muted-foreground mt-1">
        Параметры профиля и отображения портфеля
      </p>
    </div>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">Профиль</CardTitle>
        <CardDescription>Информация об учётной записи</CardDescription>
      </CardHeader>
      <CardContent class="space-y-3">
        <div v-if="auth.user" class="space-y-2 text-sm">
          <div class="flex items-center">
            <span class="w-40 text-muted-foreground shrink-0">Email</span>
            <span class="num">{{ auth.user.email }}</span>
          </div>
          <div class="flex items-center">
            <span class="w-40 text-muted-foreground shrink-0">Валюта профиля</span>
            <span class="num">{{ auth.user.displayCurrency }}</span>
          </div>
          <div class="flex items-center">
            <span class="w-40 text-muted-foreground shrink-0">Создан</span>
            <span>{{ formatDate(auth.user.createdAt) }}</span>
          </div>
          <div class="flex items-center">
            <span class="w-40 text-muted-foreground shrink-0">ID</span>
            <span class="num text-xs text-muted-foreground break-all">
              {{ auth.user.id }}
            </span>
          </div>
        </div>
        <p
          class="text-xs text-muted-foreground pt-3 mt-2 border-t border-border"
        >
          Смена email и пароля пока не реализована.
        </p>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">Внешний вид</CardTitle>
        <CardDescription>Тема и валюта отображения</CardDescription>
      </CardHeader>
      <CardContent class="space-y-5">
        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-sm font-medium">Тема</div>
            <div class="text-xs text-muted-foreground mt-0.5">
              Светлая или тёмная
            </div>
          </div>
          <div
            class="flex border border-border rounded-sm p-px bg-panel shrink-0"
          >
            <button
              type="button"
              class="px-2.5 py-1 rounded-sm text-xs cursor-pointer border-none"
              :class="
                ui.theme === 'light'
                  ? 'bg-soft text-foreground font-medium'
                  : 'bg-transparent text-muted-foreground'
              "
              @click="ui.theme = 'light'"
            >
              ☀ Светлая
            </button>
            <button
              type="button"
              class="px-2.5 py-1 rounded-sm text-xs cursor-pointer border-none"
              :class="
                ui.theme === 'dark'
                  ? 'bg-soft text-foreground font-medium'
                  : 'bg-transparent text-muted-foreground'
              "
              @click="ui.theme = 'dark'"
            >
              ☾ Тёмная
            </button>
          </div>
        </div>

        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-sm font-medium">Валюта отображения</div>
            <div class="text-xs text-muted-foreground mt-0.5">
              Используется в дэшборде и портфеле
            </div>
          </div>
          <div
            class="flex border border-border rounded-sm p-px bg-panel shrink-0"
          >
            <button
              v-for="c in ui.SUPPORTED_CURRENCIES"
              :key="c"
              type="button"
              class="px-2.5 py-1 rounded-sm text-xs num cursor-pointer border-none"
              :class="
                ui.displayCurrency === c
                  ? 'bg-soft text-foreground font-medium'
                  : 'bg-transparent text-muted-foreground'
              "
              @click="ui.displayCurrency = c"
            >
              {{ c }}
            </button>
          </div>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">Отображение портфеля</CardTitle>
        <CardDescription>Приватность и группировка позиций</CardDescription>
      </CardHeader>
      <CardContent class="space-y-5">
        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-sm font-medium">Приватный режим</div>
            <div class="text-xs text-muted-foreground mt-0.5">
              Размывать денежные суммы на экране
            </div>
          </div>
          <button
            type="button"
            :title="ui.privacy ? 'Показать суммы' : 'Скрыть суммы'"
            class="relative inline-block w-9 h-5 rounded-full transition-colors duration-150 cursor-pointer border-none p-0 shrink-0"
            :class="ui.privacy ? 'bg-accent' : 'bg-subtle'"
            @click="ui.togglePrivacy"
          >
            <span
              class="absolute inset-y-0 my-auto w-3.5 h-3.5 rounded-full bg-white shadow-sm transition-all duration-150"
              :class="ui.privacy ? 'left-5' : 'left-0.5'"
            />
          </button>
        </div>

        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-sm font-medium">Объединять одинаковые позиции</div>
            <div class="text-xs text-muted-foreground mt-0.5">
              Агрегировать позиции по инструменту между аккаунтами
            </div>
          </div>
          <button
            type="button"
            :title="
              ui.mergePositions ? 'Не объединять' : 'Объединять позиции'
            "
            class="relative inline-block w-9 h-5 rounded-full transition-colors duration-150 cursor-pointer border-none p-0 shrink-0"
            :class="ui.mergePositions ? 'bg-accent' : 'bg-subtle'"
            @click="ui.toggleMergePositions"
          >
            <span
              class="absolute inset-y-0 my-auto w-3.5 h-3.5 rounded-full bg-white shadow-sm transition-all duration-150"
              :class="ui.mergePositions ? 'left-5' : 'left-0.5'"
            />
          </button>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle class="text-base">Сессия</CardTitle>
        <CardDescription>Завершить работу в этом браузере</CardDescription>
      </CardHeader>
      <CardContent>
        <Button variant="outline" @click="handleLogout">Выйти из аккаунта</Button>
      </CardContent>
    </Card>
  </div>
</template>
