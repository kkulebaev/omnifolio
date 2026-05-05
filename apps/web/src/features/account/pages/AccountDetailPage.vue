<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useGetAccount,
  useDeleteAccount,
  useDeletePosition,
  useSyncAccount,
  getGetAccountQueryKey,
  getListAccountsQueryKey,
} from "@/api/generated";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import { formatDate, formatQuantity } from "@/lib/formatters";
import { confirm } from "@/lib/confirm";
import AddPositionDialog from "../components/AddPositionDialog.vue";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();

const accountId = computed(() => route.params.id as string);

const account = useGetAccount(accountId, {
  query: {
    // Poll while sync in progress.
    refetchInterval: (q) => (q.state.data?.lastSyncStatus === "pending" ? 3000 : false),
  },
});

const deleteAccount = useDeleteAccount();
const deletePosition = useDeletePosition();
const syncAccount = useSyncAccount();

const dialogOpen = ref(false);

const isManual = computed(() => account.data.value?.type === "manual");

async function handleDeleteAccount() {
  const ok = await confirm({
    title: "Удалить аккаунт?",
    body: "Все позиции этого аккаунта будут удалены.",
    confirmText: "Удалить",
    danger: true,
  });
  if (!ok) return;
  await deleteAccount.mutateAsync({ accountId: accountId.value });
  queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
  queryClient.invalidateQueries({ queryKey: ["portfolio"] });
  router.push({ name: "accounts" });
}

async function handleDeletePosition(instrumentId: string) {
  const ok = await confirm({
    title: "Удалить позицию?",
    confirmText: "Удалить",
    danger: true,
  });
  if (!ok) return;
  await deletePosition.mutateAsync({ accountId: accountId.value, instrumentId });
  queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(accountId.value) });
  queryClient.invalidateQueries({ queryKey: ["portfolio"] });
}

async function handleSync() {
  try {
    await syncAccount.mutateAsync({ accountId: accountId.value });
  } finally {
    queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(accountId.value) });
    queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
    queryClient.invalidateQueries({ queryKey: ["portfolio"] });
  }
}

function statusLabel(s: string | null | undefined): string {
  switch (s) {
    case "pending":
      return "синхронизация…";
    case "success":
      return "ok";
    case "failed":
      return "ошибка";
    default:
      return "—";
  }
}
</script>

<template>
  <div class="space-y-6 p-6">
    <div class="flex items-center gap-3">
      <RouterLink to="/accounts" class="text-sm opacity-60 hover:underline">←</RouterLink>
      <h1 v-if="account.data.value" class="text-2xl font-semibold">
        {{ account.data.value.name }}
      </h1>
    </div>

    <p v-if="account.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="account.isError.value" class="text-sm text-red-600">Не найден</p>

    <template v-else-if="account.data.value">
      <Card>
        <CardHeader>
          <CardDescription>Тип</CardDescription>
          <CardTitle class="text-base">{{ account.data.value.type }}</CardTitle>
          <CardDescription>
            создан {{ formatDate(account.data.value.createdAt) }}
            <template v-if="!isManual">
              · sync: <strong>{{ statusLabel(account.data.value.lastSyncStatus) }}</strong>
              <template v-if="account.data.value.lastSyncedAt">
                ({{ formatDate(account.data.value.lastSyncedAt) }})
              </template>
            </template>
          </CardDescription>
          <p
            v-if="account.data.value.lastSyncError"
            class="text-sm text-red-600 mt-2"
          >
            {{ account.data.value.lastSyncError }}
          </p>
        </CardHeader>
        <CardContent class="flex flex-wrap gap-2">
          <Button v-if="isManual" @click="dialogOpen = true">Добавить позицию</Button>
          <Button
            v-else
            :disabled="syncAccount.isPending.value || account.data.value.lastSyncStatus === 'pending'"
            @click="handleSync"
          >
            {{ syncAccount.isPending.value ? "Синхронизирую…" : "Синхронизировать" }}
          </Button>
          <Button variant="outline" @click="handleDeleteAccount">Удалить аккаунт</Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Позиции</CardTitle>
        </CardHeader>
        <CardContent class="p-0">
          <p
            v-if="!account.data.value.positions.length"
            class="px-6 pb-6 text-sm opacity-60"
          >
            Позиций ещё нет.
          </p>
          <Table v-else>
            <TableHeader>
              <TableRow>
                <TableHead>Тикер</TableHead>
                <TableHead>Класс</TableHead>
                <TableHead>Валюта</TableHead>
                <TableHead class="text-right">Количество</TableHead>
                <TableHead v-if="isManual" class="text-right">Действия</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in account.data.value.positions" :key="p.instrument.id">
                <TableCell class="font-medium">{{ p.instrument.ticker }}</TableCell>
                <TableCell>{{ p.instrument.assetClass }}</TableCell>
                <TableCell>{{ p.instrument.currency }}</TableCell>
                <TableCell class="text-right">{{ formatQuantity(p.quantity) }}</TableCell>
                <TableCell v-if="isManual" class="text-right">
                  <Button
                    variant="ghost"
                    size="sm"
                    @click="handleDeletePosition(p.instrument.id)"
                  >
                    ×
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <AddPositionDialog v-if="isManual" v-model:open="dialogOpen" :account-id="accountId" />
    </template>
  </div>
</template>
