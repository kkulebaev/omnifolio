<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useListInstruments } from "@/api/generated";
import { AssetClass } from "@/api/generated/model/assetClass";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { formatNumber } from "@/lib/formatters";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";

const PAGE_SIZE = 50;

const search = ref("");
const debouncedQ = ref("");
const offset = ref(0);
const assetClass = ref<AssetClass | "">("");

let debounceTimer: ReturnType<typeof setTimeout> | undefined;
watch(search, (v) => {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    debouncedQ.value = v.trim();
    offset.value = 0;
  }, 300);
});

watch(assetClass, () => {
  offset.value = 0;
});

const params = computed(() => ({
  q: debouncedQ.value || undefined,
  assetClass: (assetClass.value as AssetClass) || undefined,
  limit: PAGE_SIZE,
  offset: offset.value,
}));

const query = useListInstruments(params, {
  query: {
    queryKey: computed(
      () =>
        [
          "instruments",
          debouncedQ.value,
          assetClass.value,
          offset.value,
        ] as const,
    ),
    placeholderData: (prev) => prev,
  },
});

const items = computed(() => query.data.value?.items ?? []);
const total = computed(() => query.data.value?.total ?? 0);
const from = computed(() => (items.value.length ? offset.value + 1 : 0));
const to = computed(() => offset.value + items.value.length);
const canPrev = computed(() => offset.value > 0);
const canNext = computed(() => offset.value + PAGE_SIZE < total.value);

const CLASS_LABEL: Record<AssetClass, string> = {
  ru_stock: "ru акция",
  us_stock: "us акция",
  ru_bond: "ru обл.",
  ru_etf: "ru etf",
  us_etf: "us etf",
  crypto: "crypto",
  cash: "cash",
};

const FILTERS: { value: AssetClass | ""; label: string }[] = [
  { value: "", label: "Все" },
  { value: AssetClass.ru_stock, label: "RU акции" },
  { value: AssetClass.us_stock, label: "US акции" },
  { value: AssetClass.ru_bond, label: "RU облигации" },
  { value: AssetClass.ru_etf, label: "RU ETF" },
  { value: AssetClass.us_etf, label: "US ETF" },
  { value: AssetClass.crypto, label: "Крипта" },
];

function prevPage() {
  if (canPrev.value) offset.value = Math.max(0, offset.value - PAGE_SIZE);
}
function nextPage() {
  if (canNext.value) offset.value += PAGE_SIZE;
}
</script>

<template>
  <div class="space-y-4 p-6">
    <div class="flex items-center justify-between gap-4">
      <h1 class="text-2xl font-semibold">Инструменты</h1>
      <span class="text-sm text-muted-foreground num">
        {{ total }} всего
      </span>
    </div>

    <div class="flex items-center gap-3">
      <Input
        v-model="search"
        placeholder="Поиск по тикеру или названию…"
        class="max-w-md"
      />
    </div>

    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="f in FILTERS"
        :key="f.value || 'all'"
        type="button"
        class="px-3 py-1 rounded-full text-xs border cursor-pointer"
        :class="
          assetClass === f.value
            ? 'border-accent bg-soft text-foreground'
            : 'border-border bg-transparent text-muted-foreground hover:text-foreground'
        "
        @click="assetClass = f.value"
      >
        {{ f.label }}
      </button>
    </div>

    <p v-if="query.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="query.isError.value" class="text-sm text-red-600">
      Не удалось загрузить
    </p>

    <div
      v-else-if="!items.length"
      class="border border-border rounded-md bg-panel px-6 py-12 text-center text-sm text-muted-foreground"
    >
      <template v-if="debouncedQ || assetClass">Ничего не найдено</template>
      <template v-else>В каталоге пока пусто</template>
    </div>

    <div v-else class="border border-border rounded-md overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Тикер</TableHead>
            <TableHead>Название</TableHead>
            <TableHead>Класс</TableHead>
            <TableHead>Валюта</TableHead>
            <TableHead class="text-right">Цена</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="i in items" :key="i.id">
            <TableCell class="font-medium num">{{ i.ticker }}</TableCell>
            <TableCell class="text-muted-foreground">{{ i.name }}</TableCell>
            <TableCell>
              <span
                class="num uppercase text-xs px-2 py-0.5 rounded bg-soft tracking-wider"
              >
                {{ CLASS_LABEL[i.assetClass] ?? i.assetClass }}
              </span>
            </TableCell>
            <TableCell class="num">{{ i.currency }}</TableCell>
            <TableCell class="num text-right">
              <span v-if="i.currentPrice">{{ formatNumber(i.currentPrice, 2) }}</span>
              <span v-else class="text-muted-foreground opacity-50">—</span>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <div
      v-if="items.length"
      class="flex items-center justify-between text-sm text-muted-foreground"
    >
      <span class="num">{{ from }}–{{ to }} из {{ total }}</span>
      <div class="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="!canPrev || query.isFetching.value"
          @click="prevPage"
        >
          ← Назад
        </Button>
        <Button
          variant="outline"
          size="sm"
          :disabled="!canNext || query.isFetching.value"
          @click="nextPage"
        >
          Вперёд →
        </Button>
      </div>
    </div>
  </div>
</template>
