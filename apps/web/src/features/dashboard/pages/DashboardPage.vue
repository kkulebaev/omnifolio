<script setup lang="ts">
import { computed } from "vue";
import { useGetPortfolio } from "@/api/generated";
import { useUiStore } from "@/stores/ui";
import { formatCurrency, formatQuantity } from "@/lib/formatters";
import { Card, CardHeader, CardTitle, CardContent, CardDescription } from "@/components/ui/card";
import { Table, TableHeader, TableBody, TableHead, TableRow, TableCell } from "@/components/ui/table";

const ui = useUiStore();

const portfolio = useGetPortfolio(
  computed(() => ({ currency: ui.displayCurrency })),
  {
    query: {
      queryKey: computed(() => ["portfolio", ui.displayCurrency] as const),
    },
  },
);

const summary = computed(() => portfolio.data.value?.summary);
const positions = computed(() => {
  const list = portfolio.data.value?.positions ?? [];
  return [...list].sort((a, b) => Number(b.valueDisplay ?? 0) - Number(a.valueDisplay ?? 0));
});

const staleCount = computed(() => positions.value.filter((p) => p.priceStale).length);

const topAssetClass = computed(() => {
  const map = summary.value?.byAssetClass ?? {};
  let best: [string, number] | null = null;
  for (const [k, v] of Object.entries(map)) {
    const num = Number(v);
    if (!best || num > best[1]) best = [k, num];
  }
  return best;
});
</script>

<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-semibold">Дашборд</h1>

    <p v-if="portfolio.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="portfolio.isError.value" class="text-sm text-red-600">
      Не удалось загрузить портфель
    </p>

    <template v-else>
      <div class="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardDescription>Всего</CardDescription>
            <CardTitle>{{ formatCurrency(summary?.grandTotal ?? "0", ui.displayCurrency) }}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Топ класс активов</CardDescription>
            <CardTitle v-if="topAssetClass">
              {{ topAssetClass[0] }} —
              {{ formatCurrency(topAssetClass[1], ui.displayCurrency) }}
            </CardTitle>
            <CardTitle v-else>—</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Устаревшие цены</CardDescription>
            <CardTitle>{{ staleCount }}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card v-if="positions.length === 0">
        <CardContent class="py-12 text-center space-y-2">
          <p class="text-sm opacity-60">Нет позиций</p>
          <p class="text-sm">
            Зайди в
            <RouterLink to="/accounts" class="underline">Аккаунты</RouterLink>
            и добавь первую.
          </p>
        </CardContent>
      </Card>

      <Card v-else>
        <CardContent class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Тикер</TableHead>
                <TableHead>Класс</TableHead>
                <TableHead>Аккаунт</TableHead>
                <TableHead class="text-right">Количество</TableHead>
                <TableHead class="text-right">Цена</TableHead>
                <TableHead class="text-right">Стоимость</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="p in positions" :key="`${p.accountId}/${p.instrumentId}`">
                <TableCell class="font-medium">{{ p.ticker }}</TableCell>
                <TableCell>{{ p.assetClass }}</TableCell>
                <TableCell>{{ p.accountName }}</TableCell>
                <TableCell class="text-right">{{ formatQuantity(p.quantity) }}</TableCell>
                <TableCell class="text-right">
                  <span v-if="p.price">{{ formatCurrency(p.price, p.currency) }}</span>
                  <span v-else class="opacity-50">—</span>
                </TableCell>
                <TableCell class="text-right">
                  <span v-if="p.valueDisplay">
                    {{ formatCurrency(p.valueDisplay, ui.displayCurrency) }}
                  </span>
                  <span v-else class="opacity-50" :title="p.priceStale ? 'Цена устарела' : ''">—</span>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </template>
  </div>
</template>
