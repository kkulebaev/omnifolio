<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useListDeposits,
  useDeleteDeposit,
  getListDepositsQueryKey,
} from "@/api/generated";
import type { Deposit } from "@/api/generated/model/deposit";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { formatCurrency } from "@/lib/formatters";
import { confirm } from "@/lib/confirm";
import { Trash2, Plus, ChevronDown } from "lucide-vue-next";
import CreateDepositDialog from "../components/CreateDepositDialog.vue";

const list = useListDeposits();
const deleteDeposit = useDeleteDeposit();
const queryClient = useQueryClient();
const dialogOpen = ref(false);
const collapsedYears = ref<Set<number>>(new Set());
let collapseInitialized = false;

function toggleYear(year: number) {
  const next = new Set(collapsedYears.value);
  if (next.has(year)) next.delete(year);
  else next.add(year);
  collapsedYears.value = next;
}

const monthFormatter = new Intl.DateTimeFormat("ru-RU", {
  month: "long",
  year: "numeric",
});

function formatMonth(d: string): string {
  // d is "YYYY-MM-01"; build a Date as local to avoid TZ shift.
  const [y, m] = d.split("-").map(Number);
  const dt = new Date(y!, (m ?? 1) - 1, 1);
  const s = monthFormatter.format(dt);
  return s.charAt(0).toUpperCase() + s.slice(1).replace(" г.", "");
}

const lifetimeTotal = computed(() => {
  const items = list.data.value?.items ?? [];
  return items.reduce((acc, d) => acc + Number(d.amount), 0);
});

interface YearGroup {
  year: number;
  subtotal: number;
  items: Deposit[];
}

const groups = computed<YearGroup[]>(() => {
  const items = list.data.value?.items ?? [];
  const map = new Map<number, YearGroup>();
  for (const d of items) {
    const year = Number(d.month.slice(0, 4));
    let g = map.get(year);
    if (!g) {
      g = { year, subtotal: 0, items: [] };
      map.set(year, g);
    }
    g.items.push(d);
    g.subtotal += Number(d.amount);
  }
  return [...map.values()].sort((a, b) => b.year - a.year);
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
    title: "Удалить пополнение?",
    body: `${formatMonth(d.month)} — ${formatCurrency(d.amount, "RUB")}`,
    confirmText: "Удалить",
    danger: true,
  });
  if (!ok) return;
  await deleteDeposit.mutateAsync({ depositId: d.id });
  queryClient.invalidateQueries({ queryKey: getListDepositsQueryKey() });
}
</script>

<template>
  <div class="space-y-4 sm:space-y-6 p-4 sm:p-6">
    <div class="flex items-center justify-between gap-3">
      <h1 class="text-xl sm:text-2xl font-semibold">Пополнения</h1>
      <Button @click="dialogOpen = true">
        <Plus :size="16" :stroke-width="1.75" />
        Добавить
      </Button>
    </div>

    <p v-if="list.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="list.isError.value" class="text-sm text-red-600">
      Не удалось загрузить
    </p>

    <template v-else>
      <Card v-if="(list.data.value?.items ?? []).length > 0">
        <CardContent class="py-4">
          <div class="flex items-baseline justify-between gap-3">
            <span class="text-sm text-muted-foreground">Σ за всё время</span>
            <span class="text-xl font-semibold num">
              {{ formatCurrency(lifetimeTotal, "RUB") }}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card
        v-if="(list.data.value?.items ?? []).length === 0"
      >
        <CardContent class="py-10 text-center text-sm text-muted-foreground">
          Пока ни одного пополнения.
          <button
            type="button"
            class="text-foreground underline hover:no-underline ml-1 cursor-pointer bg-transparent border-none p-0"
            @click="dialogOpen = true"
          >
            Добавить первое.
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
            <span class="text-sm font-medium num">
              {{ formatCurrency(g.subtotal, "RUB") }}
            </span>
          </div>
        </CardHeader>
        <CardContent v-if="!collapsedYears.has(g.year)" class="p-0">
          <ul class="divide-y divide-border">
            <li
              v-for="d in g.items"
              :key="d.id"
              class="flex items-center justify-between gap-3 px-4 sm:px-6 py-3"
            >
              <span class="text-sm">{{ formatMonth(d.month) }}</span>
              <div class="flex items-center gap-2">
                <span class="text-sm num">{{ formatCurrency(d.amount, "RUB") }}</span>
                <Button
                  variant="ghost"
                  size="sm"
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

    <CreateDepositDialog v-model:open="dialogOpen" />
  </div>
</template>
