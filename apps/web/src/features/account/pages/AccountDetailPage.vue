<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useGetAccount,
  useDeleteAccount,
  useDeletePosition,
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
import AddPositionDialog from "../components/AddPositionDialog.vue";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();

const accountId = computed(() => route.params.id as string);
const account = useGetAccount(accountId);

const deleteAccount = useDeleteAccount();
const deletePosition = useDeletePosition();

const dialogOpen = ref(false);

async function handleDeleteAccount() {
  if (!confirm("Удалить аккаунт со всеми позициями?")) return;
  await deleteAccount.mutateAsync({ accountId: accountId.value });
  queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
  queryClient.invalidateQueries({ queryKey: ["portfolio"] });
  router.push({ name: "accounts" });
}

async function handleDeletePosition(instrumentId: string) {
  if (!confirm("Удалить позицию?")) return;
  await deletePosition.mutateAsync({ accountId: accountId.value, instrumentId });
  queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(accountId.value) });
  queryClient.invalidateQueries({ queryKey: ["portfolio"] });
}
</script>

<template>
  <div class="space-y-6">
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
          <CardDescription>создан {{ formatDate(account.data.value.createdAt) }}</CardDescription>
        </CardHeader>
        <CardContent class="flex gap-2">
          <Button @click="dialogOpen = true">Добавить позицию</Button>
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
                <TableHead class="text-right">Действия</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in account.data.value.positions" :key="p.instrument.id">
                <TableCell class="font-medium">{{ p.instrument.ticker }}</TableCell>
                <TableCell>{{ p.instrument.assetClass }}</TableCell>
                <TableCell>{{ p.instrument.currency }}</TableCell>
                <TableCell class="text-right">{{ formatQuantity(p.quantity) }}</TableCell>
                <TableCell class="text-right">
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

      <AddPositionDialog v-model:open="dialogOpen" :account-id="accountId" />
    </template>
  </div>
</template>
