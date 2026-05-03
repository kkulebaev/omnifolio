<script setup lang="ts">
import { ref } from "vue";
import { useListAccounts } from "@/api/generated";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/formatters";
import CreateAccountDialog from "../components/CreateAccountDialog.vue";

const accounts = useListAccounts();
const dialogOpen = ref(false);

function statusBadge(s: string | null | undefined): { label: string; cls: string } | null {
  switch (s) {
    case "pending":
      return { label: "синхр…", cls: "bg-yellow-200 text-yellow-900" };
    case "success":
      return { label: "ok", cls: "bg-green-200 text-green-900" };
    case "failed":
      return { label: "ошибка", cls: "bg-red-200 text-red-900" };
    default:
      return null;
  }
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold">Аккаунты</h1>
      <Button @click="dialogOpen = true">Создать аккаунт</Button>
    </div>

    <p v-if="accounts.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="accounts.isError.value" class="text-sm text-red-600">Не удалось загрузить</p>

    <Card v-else-if="!accounts.data.value?.items?.length">
      <CardContent class="py-12 text-center space-y-2">
        <p class="text-sm opacity-60">Пока нет аккаунтов</p>
        <Button @click="dialogOpen = true">Создать первый аккаунт</Button>
      </CardContent>
    </Card>

    <div v-else class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <RouterLink
        v-for="a in accounts.data.value!.items"
        :key="a.id"
        :to="{ name: 'account-detail', params: { id: a.id } }"
        class="block"
      >
        <Card class="hover:bg-muted/30 transition">
          <CardHeader>
            <div class="flex items-start justify-between gap-2">
              <CardTitle>{{ a.name }}</CardTitle>
              <span
                v-if="statusBadge(a.lastSyncStatus)"
                class="text-xs px-2 py-0.5 rounded-full"
                :class="statusBadge(a.lastSyncStatus)!.cls"
              >
                {{ statusBadge(a.lastSyncStatus)!.label }}
              </span>
            </div>
            <CardDescription>
              {{ a.type }} · создан {{ formatDate(a.createdAt) }}
            </CardDescription>
          </CardHeader>
        </Card>
      </RouterLink>
    </div>

    <CreateAccountDialog v-model:open="dialogOpen" />
  </div>
</template>
