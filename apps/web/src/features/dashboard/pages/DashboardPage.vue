<script setup lang="ts">
import { computed } from "vue";
import { useGetPortfolio } from "@/api/generated";
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

const summary = computed(() => portfolio.data.value?.summary);
const positionsRaw = computed(() => portfolio.data.value?.positions ?? []);
const positions = computed(() =>
  [...positionsRaw.value].sort(
    (a, b) => Number(b.valueDisplay ?? 0) - Number(a.valueDisplay ?? 0),
  ),
);

const grandTotal = computed(() => Number(summary.value?.grandTotal ?? 0) || 0);

const SUMMARY_CLASSES: AssetClass[] = ["ru_stock", "us_stock", "crypto"];

const CLASS_LABEL: Record<AssetClass, string> = {
  ru_stock: "Российские акции",
  us_stock: "Американские акции",
  crypto: "Криптовалюты",
  ru_bond: "Российские облигации",
  ru_etf: "Российские ETF",
  us_etf: "Американские ETF",
};

const CLASS_BADGE: Record<AssetClass, { label: string; tintClass: string }> = {
  ru_stock: { label: "ru акция", tintClass: "bg-[rgba(201,100,66,0.12)]" },
  us_stock: { label: "us акция", tintClass: "bg-[rgba(90,130,180,0.14)]" },
  crypto: { label: "crypto", tintClass: "bg-[rgba(120,150,90,0.14)]" },
  ru_bond: { label: "ru обл", tintClass: "bg-[rgba(140,120,180,0.14)]" },
  ru_etf: { label: "ru etf", tintClass: "bg-[rgba(180,150,90,0.14)]" },
  us_etf: { label: "us etf", tintClass: "bg-[rgba(70,140,160,0.14)]" },
};

const summaryCards = computed(() =>
  SUMMARY_CLASSES.map((k) => {
    const value = Number(summary.value?.byAssetClass?.[k] ?? 0) || 0;
    return {
      key: k,
      label: CLASS_LABEL[k],
      value,
      share: grandTotal.value > 0 ? value / grandTotal.value : 0,
    };
  }),
);

const accountCount = computed(
  () => Object.keys(summary.value?.byAccount ?? {}).length,
);

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
</script>

<template>
  <!-- TODO(tw-arb): spacing p-[24px] px-[12px|22px|24px|7px|8px] py-[2px|6px|7px|20px|48px] pt-[16px] pb-[28px] mt-[6px|8px] mb-[6px|8px|10px] ml-[6px] gap-[6px] | size w-[40px] min-w-[32px] h-[2px] h-[3px] | radius rounded-[1px|2px|3px|6px] | text text-[10px|10.5px|11px|11.5px|12px|12.5px|20px|30px] tracking-[-0.015em|-0.025em|0.05em|0.08em] leading-[1.05] | grid grid-cols-[minmax(280px,1.6fr)_1fr_1fr_1fr] | color text-[hsl(28_80%_52%)] bg-[rgba(...)]×6 in CLASS_BADGE -->
  <div v-if="portfolio.isLoading.value" class="p-[24px] opacity-60">
    Загрузка…
  </div>
  <div v-else-if="portfolio.isError.value" class="p-[24px] text-neg">
    Не удалось загрузить портфель
  </div>
  <template v-else>
    <div
      class="grid grid-cols-[minmax(280px,1.6fr)_1fr_1fr_1fr] border-b border-border"
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
        <div class="flex gap-[6px] text-[11px] text-muted-foreground">
          <span class="px-[8px] py-[2px] rounded-[3px] border border-border bg-panel">Все классы</span>
          <span class="px-[8px] py-[2px] rounded-[3px] border border-border bg-panel">Все аккаунты</span>
        </div>
      </div>

      <div
        v-if="positions.length === 0"
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
              :key="`${p.accountId}/${p.instrumentId}`"
              :class="i ? 'border-t border-border' : ''"
            >
              <td class="num px-[12px] py-[6px] font-semibold text-[11.5px]">{{ p.ticker }}</td>
              <td class="px-[12px] py-[6px]">
                <span
                  class="num uppercase text-[10px] px-[7px] py-[2px] rounded-[3px] text-foreground tracking-[0.05em]"
                  :class="CLASS_BADGE[p.assetClass]?.tintClass ?? 'bg-soft'"
                >{{ CLASS_BADGE[p.assetClass]?.label ?? p.assetClass }}</span>
              </td>
              <td class="px-[12px] py-[6px] text-muted-foreground text-[11.5px]">{{ p.accountName }}</td>
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
