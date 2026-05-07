<script setup lang="ts">
import { computed, ref } from "vue";
import {
  DropdownMenuRoot,
  DropdownMenuTrigger,
  DropdownMenuPortal,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItemIndicator,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "radix-vue";
import { ChevronDown, Check, Layers } from "lucide-vue-next";
import { useGetPortfolio } from "@/api/generated";
import type { PortfolioPosition } from "@/api/generated/model/portfolioPosition";
import { useUiStore } from "@/stores/ui";
import PortfolioHistoryChart from "@/features/dashboard/components/PortfolioHistoryChart.vue";
import {
  formatCompact,
  formatCurrency,
  formatDate,
  formatNumber,
  formatQuantity,
  formatRelative,
} from "@/lib/formatters";
import { pluralRu } from "@/lib/plural";
import type { AssetClass } from "@/api/generated/model/assetClass";

const ui = useUiStore();

const portfolio = useGetPortfolio(
  computed(() => ({ currency: ui.displayCurrency })),
  {
    query: {
      queryKey: computed(() => ["portfolio", ui.displayCurrency] as const),
    },
  },
);

const positionsRaw = computed(() => portfolio.data.value?.positions ?? []);

const selectedClasses = ref<Set<AssetClass>>(new Set());
const selectedAccounts = ref<Set<string>>(new Set());

function toggleClass(c: AssetClass, on: boolean) {
  const next = new Set(selectedClasses.value);
  if (on) next.add(c);
  else next.delete(c);
  selectedClasses.value = next;
}
function toggleAccount(id: string, on: boolean) {
  const next = new Set(selectedAccounts.value);
  if (on) next.add(id);
  else next.delete(id);
  selectedAccounts.value = next;
}
function clearClasses() {
  selectedClasses.value = new Set();
}
function clearAccounts() {
  selectedAccounts.value = new Set();
}
function clearAll() {
  clearClasses();
  clearAccounts();
}

const availableClasses = computed(() => {
  const set = new Set<AssetClass>();
  for (const p of positionsRaw.value) set.add(p.assetClass);
  return Array.from(set);
});

const availableAccounts = computed(() => {
  const map = new Map<string, string>();
  for (const p of positionsRaw.value) map.set(p.accountId, p.accountName);
  return Array.from(map, ([id, name]) => ({ id, name })).sort((a, b) =>
    a.name.localeCompare(b.name),
  );
});

const filtered = computed(() =>
  positionsRaw.value.filter((p) => {
    if (
      selectedClasses.value.size > 0 &&
      !selectedClasses.value.has(p.assetClass)
    )
      return false;
    if (
      selectedAccounts.value.size > 0 &&
      !selectedAccounts.value.has(p.accountId)
    )
      return false;
    return true;
  }),
);

type DisplayPosition = PortfolioPosition & {
  accountCount: number;
  isMerged: boolean;
};

const merged = computed<DisplayPosition[]>(() => {
  if (!ui.mergePositions) {
    return filtered.value.map((p) => ({
      ...p,
      accountCount: 1,
      isMerged: false,
    }));
  }
  const groups = new Map<string, PortfolioPosition[]>();
  for (const p of filtered.value) {
    const key = p.instrumentId;
    const arr = groups.get(key);
    if (arr) arr.push(p);
    else groups.set(key, [p]);
  }
  const out: DisplayPosition[] = [];
  for (const group of groups.values()) {
    const head = group[0];
    if (!head) continue;
    if (group.length === 1) {
      out.push({ ...head, accountCount: 1, isMerged: false });
      continue;
    }
    const qty = group.reduce((s, p) => s + Number(p.quantity ?? 0), 0);
    const valueNative = group.reduce(
      (s, p) => s + Number(p.valueNative ?? 0),
      0,
    );
    const valueDisplay = group.reduce(
      (s, p) => s + Number(p.valueDisplay ?? 0),
      0,
    );
    const oldestFetchedAt = group.reduce<string | null>((acc, p) => {
      if (!p.priceFetchedAt) return acc;
      if (!acc) return p.priceFetchedAt;
      return new Date(p.priceFetchedAt).getTime() <
        new Date(acc).getTime()
        ? p.priceFetchedAt
        : acc;
    }, null);
    out.push({
      accountId: group.map((p) => p.accountId).join(","),
      accountName: "",
      instrumentId: head.instrumentId,
      ticker: head.ticker,
      assetClass: head.assetClass,
      currency: head.currency,
      quantity: String(qty),
      price: head.price ?? null,
      valueNative: String(valueNative),
      valueDisplay: String(valueDisplay),
      priceFetchedAt: oldestFetchedAt,
      priceStale: group.some((p) => p.priceStale),
      accountCount: group.length,
      isMerged: true,
    });
  }
  return out;
});

const positions = computed(() =>
  [...merged.value].sort(
    (a, b) => Number(b.valueDisplay ?? 0) - Number(a.valueDisplay ?? 0),
  ),
);

const grandTotal = computed(() =>
  positions.value.reduce(
    (acc, p) => acc + (Number(p.valueDisplay ?? 0) || 0),
    0,
  ),
);

const byAssetClass = computed(() => {
  const m: Partial<Record<AssetClass, number>> = {};
  for (const p of positions.value) {
    m[p.assetClass] =
      (m[p.assetClass] ?? 0) + (Number(p.valueDisplay ?? 0) || 0);
  }
  return m;
});

const accountCount = computed(() => {
  const set = new Set<string>();
  for (const p of filtered.value) set.add(p.accountId);
  return set.size;
});

const SUMMARY_CLASSES: AssetClass[] = ["ru_stock", "us_stock", "crypto", "cash"];

const CLASS_LABEL: Record<AssetClass, string> = {
  ru_stock: "Российские акции",
  us_stock: "Американские акции",
  crypto: "Криптовалюты",
  cash: "Кэш",
  ru_bond: "Российские облигации",
  ru_etf: "Российские ETF",
  us_etf: "Американские ETF",
};

const CLASS_BADGE: Record<AssetClass, { label: string; tintClass: string }> = {
  ru_stock: { label: "ru акция", tintClass: "bg-asset-ru-stock" },
  us_stock: { label: "us акция", tintClass: "bg-asset-us-stock" },
  crypto: { label: "crypto", tintClass: "bg-asset-crypto" },
  cash: { label: "cash", tintClass: "bg-asset-cash" },
  ru_bond: { label: "ru обл", tintClass: "bg-asset-ru-bond" },
  ru_etf: { label: "ru etf", tintClass: "bg-asset-ru-etf" },
  us_etf: { label: "us etf", tintClass: "bg-asset-us-etf" },
};

const summaryCards = computed(() =>
  SUMMARY_CLASSES.map((k) => {
    const value = byAssetClass.value[k] ?? 0;
    return {
      key: k,
      label: CLASS_LABEL[k],
      value,
      share: grandTotal.value > 0 ? value / grandTotal.value : 0,
    };
  }),
);

const classFilterLabel = computed(() => {
  const n = selectedClasses.value.size;
  if (n === 0) return "Все классы";
  if (n === 1) {
    const c = selectedClasses.value.values().next().value as AssetClass;
    return CLASS_BADGE[c]?.label ?? c;
  }
  return `Классы: ${n}`;
});

const accountFilterLabel = computed(() => {
  const n = selectedAccounts.value.size;
  if (n === 0) return "Все аккаунты";
  if (n === 1) {
    const id = selectedAccounts.value.values().next().value as string;
    return availableAccounts.value.find((a) => a.id === id)?.name ?? "1";
  }
  return `Аккаунты: ${n}`;
});

function priceAgeClass(d?: string | null, stale?: boolean): string {
  if (!d || stale) return "text-neg";
  const ageSec = Math.abs(Date.now() - new Date(d).getTime()) / 1000;
  if (ageSec < 300) return "text-pos";
  if (ageSec < 1800) return "text-muted-foreground";
  if (ageSec < 7200) return "text-stale";
  return "text-neg";
}

const blurClass = computed(() => (ui.privacy ? "privacy-blur" : ""));

function shareOf(value: string | null | undefined): number {
  return grandTotal.value > 0 ? Number(value ?? 0) / grandTotal.value : 0;
}

function valueDisplaySuffix(): string {
  return ui.displayCurrency === "RUB" ? "₽" : ui.displayCurrency;
}

function pluralAccounts(n: number): string {
  return pluralRu(n, ["аккаунт", "аккаунта", "аккаунтов"]);
}

function pluralPositions(n: number): string {
  return pluralRu(n, ["позиция", "позиции", "позиций"]);
}
</script>

<template>
  <div v-if="portfolio.isLoading.value" class="p-6 opacity-60">
    Загрузка…
  </div>
  <div v-else-if="portfolio.isError.value" class="p-6 text-neg">
    Не удалось загрузить портфель
  </div>
  <template v-else>
    <!-- grid-cols arbitrary value намеренно: minmax-шаблон с одной "толстой" колонкой не выражается через стандартную шкалу Tailwind. -->
    <div
      class="md:sticky md:top-0 z-10 bg-background grid grid-cols-2 lg:grid-cols-[minmax(280px,1.6fr)_1fr_1fr_1fr_1fr] border-b border-border"
    >
      <div class="px-4 sm:px-6 py-4 sm:py-5 col-span-2 lg:col-span-1 border-b lg:border-b-0 lg:border-r border-border">
        <div class="uppercase text-xs text-muted-foreground tracking-wider mb-1.5">
          Общая стоимость
        </div>
        <div
          :class="['num', blurClass, 'text-2xl sm:text-3xl font-semibold tracking-tight leading-none']"
        >
          {{ formatCurrency(grandTotal, ui.displayCurrency) }}
        </div>
        <div class="text-xs text-muted-foreground mt-1.5">
          {{ positions.length }} {{ pluralPositions(positions.length) }} · {{ accountCount }} {{ pluralAccounts(accountCount) }} · {{ formatDate(new Date()) }}
        </div>
      </div>

      <div
        v-for="(c, i) in summaryCards"
        :key="c.key"
        class="px-4 sm:px-6 py-4 sm:py-5"
        :class="[
          i < summaryCards.length - 1 ? 'lg:border-r border-border' : '',
          i % 2 === 0 ? 'border-r border-border lg:border-r' : '',
          i < summaryCards.length - 2 ? 'border-b lg:border-b-0' : '',
        ]"
      >
        <div class="uppercase text-xs text-muted-foreground tracking-wider mb-1.5">
          {{ c.label }}
        </div>
        <div
          :class="['num', blurClass, 'text-xl font-semibold tracking-tight']"
        >
          {{ formatCompact(c.value, ui.displayCurrency) }}
        </div>
        <div class="flex items-center gap-1.5 mt-2">
          <div class="flex-1 h-0.5 bg-soft rounded-xs">
            <div
              class="h-full bg-accent rounded-xs"
              :style="{ width: `${c.share * 100}%` }"
            />
          </div>
          <span class="num text-xs text-muted-foreground">{{ (c.share * 100).toFixed(1) }}%</span>
        </div>
      </div>
    </div>

    <PortfolioHistoryChart />

    <div class="px-4 sm:px-6 pt-4 pb-7">
      <div class="flex items-center justify-between mb-2.5 gap-2 flex-wrap">
        <h2 class="text-xs font-semibold m-0">
          Позиции
          <span class="text-muted-foreground font-normal ml-1.5">
            {{ positions.length }}
          </span>
        </h2>
        <div class="flex gap-1.5 text-xs flex-wrap">
          <button
            type="button"
            :title="ui.mergePositions ? 'Развернуть по аккаунтам' : 'Агрегировать по тикеру'"
            @click="ui.toggleMergePositions()"
            class="inline-flex items-center cursor-pointer gap-2 px-2.5 py-1 border border-border rounded-sm bg-panel text-muted-foreground text-xs"
          >
            <span :class="ui.mergePositions ? 'text-foreground' : 'text-muted-foreground'">Агрегировать</span>
            <span
              class="relative inline-block w-6 h-3.5 rounded-full transition-colors duration-150"
              :class="ui.mergePositions ? 'bg-accent' : 'bg-subtle'"
            >
              <span
                class="absolute top-px w-3 h-3 rounded-full bg-white shadow-sm transition-all duration-150"
                :class="ui.mergePositions ? 'left-3' : 'left-px'"
              />
            </span>
          </button>

          <DropdownMenuRoot>
            <DropdownMenuTrigger
              class="px-2 py-0.5 rounded-sm border border-border bg-panel inline-flex items-center gap-1 hover:bg-soft transition-colors outline-none data-[state=open]:bg-soft"
              :class="selectedClasses.size > 0 ? 'text-foreground' : 'text-muted-foreground'"
            >
              {{ classFilterLabel }}
              <ChevronDown class="w-2.5 h-2.5 opacity-60" />
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent
                align="end"
                :side-offset="4"
                class="z-50 min-w-52 rounded-lg border border-border bg-panel p-1 shadow-md text-xs"
              >
                <template v-if="availableClasses.length === 0">
                  <div class="px-2 py-1 text-muted-foreground">
                    Нет классов
                  </div>
                </template>
                <DropdownMenuCheckboxItem
                  v-for="c in availableClasses"
                  :key="c"
                  :checked="selectedClasses.has(c)"
                  @update:checked="(v) => toggleClass(c, v)"
                  @select.prevent
                  class="relative flex items-center gap-2 pl-6 pr-2 py-1 rounded-sm cursor-pointer outline-none data-[highlighted]:bg-soft"
                >
                  <DropdownMenuItemIndicator class="absolute left-1.5 inline-flex items-center">
                    <Check class="w-3 h-3" />
                  </DropdownMenuItemIndicator>
                  <span
                    class="num uppercase text-xs px-2 py-0.5 rounded-sm tracking-wider"
                    :class="CLASS_BADGE[c]?.tintClass ?? 'bg-soft'"
                  >{{ CLASS_BADGE[c]?.label ?? c }}</span>
                  <span class="text-muted-foreground text-xs">{{ CLASS_LABEL[c] ?? c }}</span>
                </DropdownMenuCheckboxItem>
                <template v-if="selectedClasses.size > 0">
                  <DropdownMenuSeparator class="my-1 h-px bg-border" />
                  <DropdownMenuItem
                    @select="clearClasses"
                    class="px-2 py-1 rounded-sm cursor-pointer outline-none text-muted-foreground data-[highlighted]:bg-soft"
                  >Сбросить</DropdownMenuItem>
                </template>
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenuRoot>

          <DropdownMenuRoot>
            <DropdownMenuTrigger
              class="px-2 py-0.5 rounded-sm border border-border bg-panel inline-flex items-center gap-1 hover:bg-soft transition-colors outline-none data-[state=open]:bg-soft"
              :class="selectedAccounts.size > 0 ? 'text-foreground' : 'text-muted-foreground'"
            >
              {{ accountFilterLabel }}
              <ChevronDown class="w-2.5 h-2.5 opacity-60" />
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent
                align="end"
                :side-offset="4"
                class="z-50 min-w-52 rounded-lg border border-border bg-panel p-1 shadow-md text-xs"
              >
                <template v-if="availableAccounts.length === 0">
                  <div class="px-2 py-1 text-muted-foreground">
                    Нет аккаунтов
                  </div>
                </template>
                <DropdownMenuCheckboxItem
                  v-for="a in availableAccounts"
                  :key="a.id"
                  :checked="selectedAccounts.has(a.id)"
                  @update:checked="(v) => toggleAccount(a.id, v)"
                  @select.prevent
                  class="relative flex items-center gap-2 pl-6 pr-2 py-1 rounded-sm cursor-pointer outline-none data-[highlighted]:bg-soft"
                >
                  <DropdownMenuItemIndicator class="absolute left-1.5 inline-flex items-center">
                    <Check class="w-3 h-3" />
                  </DropdownMenuItemIndicator>
                  <span>{{ a.name }}</span>
                </DropdownMenuCheckboxItem>
                <template v-if="selectedAccounts.size > 0">
                  <DropdownMenuSeparator class="my-1 h-px bg-border" />
                  <DropdownMenuItem
                    @select="clearAccounts"
                    class="px-2 py-1 rounded-sm cursor-pointer outline-none text-muted-foreground data-[highlighted]:bg-soft"
                  >Сбросить</DropdownMenuItem>
                </template>
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenuRoot>
        </div>
      </div>

      <div
        v-if="positions.length === 0 && positionsRaw.length === 0"
        class="border border-border rounded-lg bg-panel px-6 py-12 text-center"
      >
        <p class="text-xs text-muted-foreground mt-0 mx-0 mb-2">Нет позиций</p>
        <p class="m-0 text-xs">
          Зайди в
          <RouterLink to="/accounts" class="text-accent underline">Аккаунты</RouterLink>
          и добавь первую.
        </p>
      </div>

      <div
        v-else-if="positions.length === 0"
        class="border border-border rounded-lg bg-panel px-6 py-12 text-center"
      >
        <p class="text-xs text-muted-foreground mt-0 mx-0 mb-2">Нет позиций под выбранные фильтры</p>
        <button
          type="button"
          class="text-xs text-accent underline cursor-pointer bg-transparent border-none p-0"
          @click="clearAll"
        >Сбросить фильтры</button>
      </div>

      <div
        v-else
        class="border border-border rounded-lg overflow-hidden bg-panel md:hidden divide-y divide-border"
      >
        <div
          v-for="p in positions"
          :key="p.isMerged ? `m-mob/${p.instrumentId}` : `mob/${p.accountId}/${p.instrumentId}`"
          class="px-3 py-2.5"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0">
              <span class="num font-semibold text-xs">{{ p.ticker }}</span>
              <span
                class="num uppercase text-xs px-1.5 py-0.5 rounded-sm text-foreground tracking-wider shrink-0"
                :class="CLASS_BADGE[p.assetClass]?.tintClass ?? 'bg-soft'"
              >{{ CLASS_BADGE[p.assetClass]?.label ?? p.assetClass }}</span>
            </div>
            <span
              :class="['num', blurClass, 'text-xs font-medium shrink-0']"
            >
              <span v-if="p.valueDisplay">
                {{ formatNumber(p.valueDisplay, 0) }} {{ valueDisplaySuffix() }}
              </span>
              <span v-else class="opacity-50">—</span>
            </span>
          </div>
          <div class="flex items-center justify-between gap-2 mt-1.5 text-xs text-muted-foreground">
            <span class="truncate">
              <span v-if="p.isMerged" class="inline-flex items-center gap-1">
                <Layers class="w-2.5 h-2.5 opacity-60" />
                {{ p.accountCount }} {{ pluralAccounts(p.accountCount) }}
              </span>
              <span v-else>{{ p.accountName }}</span>
            </span>
            <span class="num shrink-0">
              {{ formatQuantity(p.quantity) }}
              <span v-if="p.price" class="opacity-60"> · {{ formatNumber(p.price, 2) }}</span>
            </span>
          </div>
          <div class="flex items-center gap-2 mt-1.5">
            <div class="flex-1 h-0.5 bg-soft rounded-xs">
              <div
                class="h-full bg-accent opacity-75 rounded-xs"
                :style="{ width: `${Math.min(100, shareOf(p.valueDisplay) * 4 * 100)}%` }"
              />
            </div>
            <span class="num text-xs text-muted-foreground min-w-8 text-right">{{ (shareOf(p.valueDisplay) * 100).toFixed(1) }}%</span>
            <span
              class="num text-xs shrink-0"
              :class="priceAgeClass(p.priceFetchedAt, p.priceStale)"
            >
              {{ p.priceFetchedAt ? formatRelative(p.priceFetchedAt) : "—" }}
            </span>
          </div>
        </div>
      </div>

      <div
        v-if="positions.length"
        class="hidden md:block border border-border rounded-lg overflow-hidden bg-panel"
      >
        <table class="w-full border-collapse text-xs">
          <thead>
            <tr class="bg-background">
              <th
                v-for="(h, i) in ['Тикер','Класс','Аккаунт','Кол-во','Цена','Обновлена','Стоимость','Доля']"
                :key="h"
                class="uppercase px-3 py-2 text-xs font-medium text-muted-foreground tracking-wider border-b border-border"
                :class="i >= 3 ? 'text-right' : 'text-left'"
              >{{ h }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(p, i) in positions"
              :key="p.isMerged ? `merged/${p.instrumentId}` : `${p.accountId}/${p.instrumentId}`"
              :class="i ? 'border-t border-border' : ''"
            >
              <td class="num px-3 py-1.5 font-semibold text-xs">{{ p.ticker }}</td>
              <td class="px-3 py-1.5">
                <span
                  class="num uppercase text-xs px-2 py-0.5 rounded-sm text-foreground tracking-wider"
                  :class="CLASS_BADGE[p.assetClass]?.tintClass ?? 'bg-soft'"
                >{{ CLASS_BADGE[p.assetClass]?.label ?? p.assetClass }}</span>
              </td>
              <td class="px-3 py-1.5 text-muted-foreground text-xs">
                <span v-if="p.isMerged" class="inline-flex items-center gap-1">
                  <Layers class="w-2.5 h-2.5 opacity-60" />
                  {{ p.accountCount }} {{ pluralAccounts(p.accountCount) }}
                </span>
                <span v-else>{{ p.accountName }}</span>
              </td>
              <td class="num px-3 py-1.5 text-right text-xs">{{ formatQuantity(p.quantity) }}</td>
              <td class="num px-3 py-1.5 text-right text-muted-foreground text-xs">
                <span v-if="p.price">{{ formatNumber(p.price, 2) }}</span>
                <span v-else class="opacity-50">—</span>
              </td>
              <td
                class="num px-3 py-1.5 text-right text-xs"
                :class="priceAgeClass(p.priceFetchedAt, p.priceStale)"
              >
                {{ p.priceFetchedAt ? formatRelative(p.priceFetchedAt) : "—" }}
              </td>
              <td
                :class="['num', blurClass, 'px-3 py-1.5 text-right font-medium text-xs']"
              >
                <span v-if="p.valueDisplay">
                  {{ formatNumber(p.valueDisplay, 0) }} {{ valueDisplaySuffix() }}
                </span>
                <span v-else class="opacity-50">—</span>
              </td>
              <td class="px-3 py-1.5 text-right">
                <div class="inline-flex items-center justify-end gap-1.5">
                  <div class="w-10 h-1 bg-soft rounded-xs">
                    <div
                      class="h-full bg-accent opacity-75 rounded-xs"
                      :style="{ width: `${Math.min(100, shareOf(p.valueDisplay) * 4 * 100)}%` }"
                    />
                  </div>
                  <span class="num text-muted-foreground text-xs min-w-8 text-right">{{ (shareOf(p.valueDisplay) * 100).toFixed(1) }}%</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </template>
</template>
