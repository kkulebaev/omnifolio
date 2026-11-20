<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import { useListDeposits, useDeleteDeposit } from "@/api/generated";
import type { Deposit } from "@/api/generated/model/deposit";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { formatCurrency, formatDay } from "@/lib/formatters";
import { confirm } from "@/lib/confirm";
import { Trash2, Plus, ChevronDown, Pencil } from "@lucide/vue";
import DepositFormDialog from "../components/DepositFormDialog.vue";
import { invalidateDepositQueries } from "../invalidate";

const list = useListDeposits();
const deleteDeposit = useDeleteDeposit();
const queryClient = useQueryClient();
const dialogOpen = ref(false);
const editing = ref<Deposit | null>(null);
const collapsedYears = ref<Set<number>>(new Set());
let collapseInitialized = false;

function toggleYear(year: number) {
  const next = new Set(collapsedYears.value);
  if (next.has(year)) next.delete(year);
  else next.add(year);
  collapsedYears.value = next;
}

function openCreate() {
  editing.value = null;
  dialogOpen.value = true;
}

function openEdit(d: Deposit) {
  editing.value = d;
  dialogOpen.value = true;
}

const isWithdrawal = (d: Deposit) => d.kind === "withdrawal";

function signedAmount(d: Deposit): number {
  return isWithdrawal(d) ? -Number(d.amount) : Number(d.amount);
}

function formatFlow(d: Deposit): string {
  const s = formatCurrency(d.amount, d.currency);
  return isWithdrawal(d) ? `−${s}` : s;
}

type CurrencyTotal = { currency: string; total: number };

// Amounts in different currencies are never summed together on the client:
// conversion needs historical rates, which only the backend has.
function netByCurrency(items: Deposit[]): CurrencyTotal[] {
  const m = new Map<string, number>();
  for (const d of items) m.set(d.currency, (m.get(d.currency) ?? 0) + signedAmount(d));
  return [...m]
    .map(([currency, total]) => ({ currency, total }))
    .sort((a, b) => a.currency.localeCompare(b.currency));
}

const items = computed(() => list.data.value?.items ?? []);
const lifetimeTotals = computed(() => netByCurrency(items.value));

interface YearGroup {
  year: number;
  totals: CurrencyTotal[];
  items: Deposit[];
}

const groups = computed<YearGroup[]>(() => {
  const map = new Map<number, Deposit[]>();
  for (const d of items.value) {
    const year = Number(d.date.slice(0, 4));
    const arr = map.get(year);
    if (arr) arr.push(d);
    else map.set(year, [d]);
  }
  return [...map]
    .map(([year, ds]) => ({ year, items: ds, totals: netByCurrency(ds) }))
    .sort((a, b) => b.year - a.year);
});

watch(
  groups,
  (g) => {
    if (collapseInitialized || g.length === 0) return;
    collapsedYears.value = new Set(g.slice(1).map((x) => x.year));
    collapseInitialized = true;
  },
  { immediate: true },
);

async function handleDelete(d: Deposit) {
  const ok = await confirm({
    title: isWithdrawal(d) ? "Удалить вывод?" : "Удалить пополнение?",
    body: `${formatDay(d.date)} — ${formatFlow(d)}`,
    confirmText: "Удалить",
    danger: true,
  });
  if (!ok) return;
  await deleteDeposit.mutateAsync({ depositId: d.id });
  invalidateDepositQueries(queryClient);
}
</script>

<template>
  <div class="space-y-4 sm:space-y-6 p-4 sm:p-6">
    <div class="flex items-center justify-between gap-3">
      <h1 class="text-xl sm:text-2xl font-semibold">Пополнения и выводы</h1>
      <Button @click="openCreate">
        <Plus :size="16" :stroke-width="1.75" />
        Добавить
      </Button>
    </div>

    <p v-if="list.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="list.isError.value" class="text-sm text-red-600">
      Не удалось загрузить
    </p>

    <template v-else>
      <Card v-if="items.length > 0">
        <CardContent class="py-4">
          <div class="flex items-baseline justify-between gap-3">
            <span class="text-sm text-muted-foreground">Σ за всё время</span>
            <div class="flex flex-col items-end gap-0.5">
              <span
                v-for="t in lifetimeTotals"
                :key="t.currency"
                class="text-xl font-semibold num"
              >
                {{ formatCurrency(t.total, t.currency) }}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card v-if="items.length === 0">
        <CardContent class="py-10 text-center text-sm text-muted-foreground">
          Пока ни одной записи.
          <button
            type="button"
            class="text-foreground underline hover:no-underline ml-1 cursor-pointer bg-transparent border-none p-0"
            @click="openCreate"
          >
            Добавить первую.
          </button>
        </CardContent>
      </Card>

      <Card v-for="g in groups" :key="g.year">
        <CardHeader
          class="cursor-pointer select-none"
          role="button"
          tabindex="0"
          :aria-expanded="!collapsedYears.has(g.year)"
          @click="toggleYear(g.year)"
          @keydown.enter.prevent="toggleYear(g.year)"
          @keydown.space.prevent="toggleYear(g.year)"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <ChevronDown
                :size="16"
                :stroke-width="1.75"
                class="text-muted-foreground transition-transform"
                :class="collapsedYears.has(g.year) ? '-rotate-90' : ''"
              />
              <CardTitle class="text-base num">{{ g.year }}</CardTitle>
            </div>
            <div class="flex flex-col items-end">
              <span
                v-for="t in g.totals"
                :key="t.currency"
                class="text-sm font-medium num"
              >
                {{ formatCurrency(t.total, t.currency) }}
              </span>
            </div>
          </div>
        </CardHeader>
        <CardContent v-if="!collapsedYears.has(g.year)" class="p-0">
          <ul class="divide-y divide-border">
            <li
              v-for="d in g.items"
              :key="d.id"
              class="flex items-center justify-between gap-3 px-4 sm:px-6 py-3"
            >
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-sm num">{{ formatDay(d.date) }}</span>
                <span
                  v-if="isWithdrawal(d)"
                  class="text-xs px-1.5 py-0.5 rounded-sm bg-soft text-muted-foreground"
                >
                  вывод
                </span>
              </div>
              <div class="flex items-center gap-1 shrink-0">
                <span
                  class="text-sm num mr-1"
                  :class="isWithdrawal(d) ? 'text-neg' : ''"
                >
                  {{ formatFlow(d) }}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Редактировать"
                  @click="openEdit(d)"
                >
                  <Pencil :size="14" :stroke-width="1.75" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Удалить"
                  :disabled="deleteDeposit.isPending.value"
                  @click="handleDelete(d)"
                >
                  <Trash2 :size="14" :stroke-width="1.75" />
                </Button>
              </div>
            </li>
          </ul>
        </CardContent>
      </Card>
    </template>

    <DepositFormDialog v-model:open="dialogOpen" :deposit="editing" />
  </div>
</template>
