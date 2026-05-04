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
import {
  formatCompact,
  formatCurrency,
  formatDate,
  formatNumber,
  formatQuantity,
  formatRelative,
} from "@/lib/formatters";
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
  for (const p of positions.value) set.add(p.accountId);
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
  ru_stock: { label: "ru акция", tintClass: "bg-[rgba(201,100,66,0.12)]" },
  us_stock: { label: "us акция", tintClass: "bg-[rgba(90,130,180,0.14)]" },
  crypto: { label: "crypto", tintClass: "bg-[rgba(120,150,90,0.14)]" },
  cash: { label: "cash", tintClass: "bg-[rgba(150,150,150,0.14)]" },
  ru_bond: { label: "ru обл", tintClass: "bg-[rgba(140,120,180,0.14)]" },
  ru_etf: { label: "ru etf", tintClass: "bg-[rgba(180,150,90,0.14)]" },
  us_etf: { label: "us etf", tintClass: "bg-[rgba(70,140,160,0.14)]" },
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
  if (ageSec < 7200) return "text-[hsl(28_80%_52%)]";
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
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return "аккаунт";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return "аккаунта";
  return "аккаунтов";
}
</script>

<template>
  <!-- TODO(tw-arb): spacing p-[24px] px-[12px|22px|24px|7px|8px] py-[2px|6px|7px|20px|48px] pt-[16px] pb-[28px] mt-[6px|8px] mb-[6px|8px|10px] ml-[6px] gap-[6px] | size w-[40px] min-w-[32px] h-[2px] h-[3px] | radius rounded-[1px|2px|3px|6px] | text text-[10px|10.5px|11px|11.5px|12px|12.5px|20px|30px] tracking-[-0.015em|-0.025em|0.05em|0.08em] leading-[1.05] | grid grid-cols-[minmax(280px,1.6fr)_1fr_1fr_1fr_1fr] | color text-[hsl(28_80%_52%)] bg-[rgba(...)]×7 in CLASS_BADGE -->
  <div v-if="portfolio.isLoading.value" class="p-[24px] opacity-60">
    Загрузка…
  </div>
  <div v-else-if="portfolio.isError.value" class="p-[24px] text-neg">
    Не удалось загрузить портфель
  </div>
  <template v-else>
    <div
      class="sticky top-0 z-10 bg-background grid grid-cols-[minmax(280px,1.6fr)_1fr_1fr_1fr_1fr] border-b border-border"
    >
      <div class="px-[24px] py-[20px] border-r border-border">
        <div class="uppercase text-[10.5px] text-muted-foreground tracking-[0.08em] mb-[6px]">
          Общая стоимость
        </div>
        <div
          :class="['num', blurClass, 'text-[30px] font-semibold tracking-[-0.025em] leading-[1.05]']"
        >
          {{ formatCurrency(grandTotal, ui.displayCurrency) }}
        </div>
        <div class="text-[11.5px] text-muted-foreground mt-[6px]">
          {{ positions.length }} позиций · {{ accountCount }} аккаунта · {{ formatDate(new Date()) }}
        </div>
      </div>

      <div
        v-for="(c, i) in summaryCards"
        :key="c.key"
        class="px-[22px] py-[20px]"
        :class="i < summaryCards.length - 1 ? 'border-r border-border' : ''"
      >
        <div class="uppercase text-[10.5px] text-muted-foreground tracking-[0.08em] mb-[6px]">
          {{ c.label }}
        </div>
        <div
          :class="['num', blurClass, 'text-[20px] font-semibold tracking-[-0.015em]']"
        >
          {{ formatCompact(c.value, ui.displayCurrency) }}
        </div>
        <div class="flex items-center gap-[6px] mt-[8px]">
          <div class="flex-1 h-[2px] bg-soft rounded-[1px]">
            <div
              class="h-full bg-accent rounded-[1px]"
              :style="{ width: `${c.share * 100}%` }"
            />
          </div>
          <span class="num text-[11px] text-muted-foreground">{{ (c.share * 100).toFixed(1) }}%</span>
        </div>
      </div>
    </div>

    <div class="px-[22px] pt-[16px] pb-[28px]">
      <div class="flex items-center justify-between mb-[10px]">
        <h2 class="text-[12.5px] font-semibold m-0">
          Позиции
          <span class="text-muted-foreground font-normal ml-[6px]">
            {{ positions.length }}
          </span>
        </h2>
        <div class="flex gap-[6px] text-[11px]">
          <button
            type="button"
            :title="ui.mergePositions ? 'Развернуть по аккаунтам' : 'Агрегировать по тикеру'"
            @click="ui.toggleMergePositions"
            class="inline-flex items-center cursor-pointer gap-[8px] px-[9px] py-[3px] border border-border rounded-[4px] bg-panel text-muted-foreground text-[11.5px] font-[inherit]"
          >
            <span :class="ui.mergePositions ? 'text-foreground' : 'text-muted-foreground'">Агрегировать</span>
            <span
              class="relative inline-block w-[24px] h-[13px] rounded-[7px] transition-colors duration-150"
              :class="ui.mergePositions ? 'bg-accent' : 'bg-subtle'"
            >
              <span
                class="absolute top-[1px] w-[11px] h-[11px] rounded-[6px] bg-white shadow-[0_1px_2px_rgba(0,0,0,0.2)] transition-[left] duration-150"
                :class="ui.mergePositions ? 'left-[12px]' : 'left-[1px]'"
              />
            </span>
          </button>

          <DropdownMenuRoot>
            <DropdownMenuTrigger
              class="px-[8px] py-[2px] rounded-[3px] border border-border bg-panel inline-flex items-center gap-[4px] hover:bg-soft transition-colors outline-none data-[state=open]:bg-soft"
              :class="selectedClasses.size > 0 ? 'text-foreground' : 'text-muted-foreground'"
            >
              {{ classFilterLabel }}
              <ChevronDown class="w-[10px] h-[10px] opacity-60" />
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent
                align="end"
                :side-offset="4"
                class="z-50 min-w-[200px] rounded-[6px] border border-border bg-panel p-[4px] shadow-md text-[12px]"
              >
                <template v-if="availableClasses.length === 0">
                  <div class="px-[8px] py-[4px] text-muted-foreground">
                    Нет классов
                  </div>
                </template>
                <DropdownMenuCheckboxItem
                  v-for="c in availableClasses"
                  :key="c"
                  :checked="selectedClasses.has(c)"
                  @update:checked="(v) => toggleClass(c, v)"
                  @select.prevent
                  class="relative flex items-center gap-[8px] pl-[24px] pr-[8px] py-[4px] rounded-[3px] cursor-pointer outline-none data-[highlighted]:bg-soft"
                >
                  <DropdownMenuItemIndicator class="absolute left-[6px] inline-flex items-center">
                    <Check class="w-[12px] h-[12px]" />
                  </DropdownMenuItemIndicator>
                  <span
                    class="num uppercase text-[10px] px-[7px] py-[2px] rounded-[3px] tracking-[0.05em]"
                    :class="CLASS_BADGE[c]?.tintClass ?? 'bg-soft'"
                  >{{ CLASS_BADGE[c]?.label ?? c }}</span>
                  <span class="text-muted-foreground text-[11px]">{{ CLASS_LABEL[c] ?? c }}</span>
                </DropdownMenuCheckboxItem>
                <template v-if="selectedClasses.size > 0">
                  <DropdownMenuSeparator class="my-[4px] h-[1px] bg-border" />
                  <DropdownMenuItem
                    @select="clearClasses"
                    class="px-[8px] py-[4px] rounded-[3px] cursor-pointer outline-none text-muted-foreground data-[highlighted]:bg-soft"
                  >Сбросить</DropdownMenuItem>
                </template>
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenuRoot>

          <DropdownMenuRoot>
            <DropdownMenuTrigger
              class="px-[8px] py-[2px] rounded-[3px] border border-border bg-panel inline-flex items-center gap-[4px] hover:bg-soft transition-colors outline-none data-[state=open]:bg-soft"
              :class="selectedAccounts.size > 0 ? 'text-foreground' : 'text-muted-foreground'"
            >
              {{ accountFilterLabel }}
              <ChevronDown class="w-[10px] h-[10px] opacity-60" />
            </DropdownMenuTrigger>
            <DropdownMenuPortal>
              <DropdownMenuContent
                align="end"
                :side-offset="4"
                class="z-50 min-w-[200px] rounded-[6px] border border-border bg-panel p-[4px] shadow-md text-[12px]"
              >
                <template v-if="availableAccounts.length === 0">
                  <div class="px-[8px] py-[4px] text-muted-foreground">
                    Нет аккаунтов
                  </div>
                </template>
                <DropdownMenuCheckboxItem
                  v-for="a in availableAccounts"
                  :key="a.id"
                  :checked="selectedAccounts.has(a.id)"
                  @update:checked="(v) => toggleAccount(a.id, v)"
                  @select.prevent
                  class="relative flex items-center gap-[8px] pl-[24px] pr-[8px] py-[4px] rounded-[3px] cursor-pointer outline-none data-[highlighted]:bg-soft"
                >
                  <DropdownMenuItemIndicator class="absolute left-[6px] inline-flex items-center">
                    <Check class="w-[12px] h-[12px]" />
                  </DropdownMenuItemIndicator>
                  <span>{{ a.name }}</span>
                </DropdownMenuCheckboxItem>
                <template v-if="selectedAccounts.size > 0">
                  <DropdownMenuSeparator class="my-[4px] h-[1px] bg-border" />
                  <DropdownMenuItem
                    @select="clearAccounts"
                    class="px-[8px] py-[4px] rounded-[3px] cursor-pointer outline-none text-muted-foreground data-[highlighted]:bg-soft"
                  >Сбросить</DropdownMenuItem>
                </template>
              </DropdownMenuContent>
            </DropdownMenuPortal>
          </DropdownMenuRoot>
        </div>
      </div>

      <div
        v-if="positions.length === 0 && positionsRaw.length === 0"
        class="border border-border rounded-[6px] bg-panel px-[24px] py-[48px] text-center"
      >
        <p class="text-[12.5px] text-muted-foreground mt-0 mx-0 mb-[8px]">Нет позиций</p>
        <p class="m-0 text-[12.5px]">
          Зайди в
          <RouterLink to="/accounts" class="text-accent underline">Аккаунты</RouterLink>
          и добавь первую.
        </p>
      </div>

      <div
        v-else-if="positions.length === 0"
        class="border border-border rounded-[6px] bg-panel px-[24px] py-[48px] text-center"
      >
        <p class="text-[12.5px] text-muted-foreground mt-0 mx-0 mb-[8px]">Нет позиций под выбранные фильтры</p>
        <button
          type="button"
          class="text-[12px] text-accent underline cursor-pointer bg-transparent border-none p-0"
          @click="clearAll"
        >Сбросить фильтры</button>
      </div>

      <div
        v-else
        class="border border-border rounded-[6px] overflow-hidden bg-panel"
      >
        <table class="w-full border-collapse text-[12px]">
          <thead>
            <tr class="bg-background">
              <th
                v-for="(h, i) in ['Тикер','Класс','Аккаунт','Кол-во','Цена','Обновлена','Стоимость','Доля']"
                :key="h"
                class="uppercase px-[12px] py-[7px] text-[10.5px] font-medium text-muted-foreground tracking-[0.05em] border-b border-border"
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
              <td class="num px-[12px] py-[6px] font-semibold text-[11.5px]">{{ p.ticker }}</td>
              <td class="px-[12px] py-[6px]">
                <span
                  class="num uppercase text-[10px] px-[7px] py-[2px] rounded-[3px] text-foreground tracking-[0.05em]"
                  :class="CLASS_BADGE[p.assetClass]?.tintClass ?? 'bg-soft'"
                >{{ CLASS_BADGE[p.assetClass]?.label ?? p.assetClass }}</span>
              </td>
              <td class="px-[12px] py-[6px] text-muted-foreground text-[11.5px]">
                <span v-if="p.isMerged" class="inline-flex items-center gap-[4px]">
                  <Layers class="w-[10px] h-[10px] opacity-60" />
                  {{ p.accountCount }} {{ pluralAccounts(p.accountCount) }}
                </span>
                <span v-else>{{ p.accountName }}</span>
              </td>
              <td class="num px-[12px] py-[6px] text-right text-[11.5px]">{{ formatQuantity(p.quantity) }}</td>
              <td class="num px-[12px] py-[6px] text-right text-muted-foreground text-[11.5px]">
                <span v-if="p.price">{{ formatNumber(p.price, 2) }}</span>
                <span v-else class="opacity-50">—</span>
              </td>
              <td
                class="num px-[12px] py-[6px] text-right text-[11px]"
                :class="priceAgeClass(p.priceFetchedAt, p.priceStale)"
              >
                {{ p.priceFetchedAt ? formatRelative(p.priceFetchedAt) : "—" }}
              </td>
              <td
                :class="['num', blurClass, 'px-[12px] py-[6px] text-right font-medium text-[12px]']"
              >
                <span v-if="p.valueDisplay">
                  {{ formatNumber(p.valueDisplay, 0) }} {{ valueDisplaySuffix() }}
                </span>
                <span v-else class="opacity-50">—</span>
              </td>
              <td class="px-[12px] py-[6px] text-right">
                <div class="inline-flex items-center justify-end gap-[6px]">
                  <div class="w-[40px] h-[3px] bg-soft rounded-[2px]">
                    <div
                      class="h-full bg-accent opacity-75 rounded-[2px]"
                      :style="{ width: `${Math.min(100, shareOf(p.valueDisplay) * 4 * 100)}%` }"
                    />
                  </div>
                  <span class="num text-muted-foreground text-[10.5px] min-w-[32px] text-right">{{ (shareOf(p.valueDisplay) * 100).toFixed(1) }}%</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </template>
</template>
