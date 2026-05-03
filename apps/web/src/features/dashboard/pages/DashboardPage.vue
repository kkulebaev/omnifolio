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

const CLASS_BADGE: Record<AssetClass, { label: string; tint: string }> = {
  ru_stock: { label: "ru акция", tint: "rgba(201,100,66,0.12)" },
  us_stock: { label: "us акция", tint: "rgba(90,130,180,0.14)" },
  crypto: { label: "crypto", tint: "rgba(120,150,90,0.14)" },
  ru_bond: { label: "ru обл", tint: "rgba(140,120,180,0.14)" },
  ru_etf: { label: "ru etf", tint: "rgba(180,150,90,0.14)" },
  us_etf: { label: "us etf", tint: "rgba(70,140,160,0.14)" },
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

function priceAgeStyle(d?: string | null, stale?: boolean): { color: string } {
  if (!d || stale) return { color: "hsl(var(--neg))" };
  const ageSec = Math.abs(Date.now() - new Date(d).getTime()) / 1000;
  if (ageSec < 300) return { color: "hsl(var(--pos))" };
  if (ageSec < 1800) return { color: "hsl(var(--muted-foreground))" };
  if (ageSec < 7200) return { color: "hsl(28 80% 52%)" };
  return { color: "hsl(var(--neg))" };
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
  <div v-if="portfolio.isLoading.value" style="padding: 24px; opacity: 0.6">
    Загрузка…
  </div>
  <div
    v-else-if="portfolio.isError.value"
    style="padding: 24px; color: hsl(var(--neg))"
  >
    Не удалось загрузить портфель
  </div>
  <template v-else>
    <div
      class="grid"
      style="grid-template-columns: minmax(280px, 1.6fr) 1fr 1fr 1fr; border-bottom: 1px solid hsl(var(--border));"
    >
      <div style="padding: 20px 24px; border-right: 1px solid hsl(var(--border));">
        <div
          class="uppercase"
          style="font-size: 10.5px; color: hsl(var(--muted-foreground)); letter-spacing: 0.08em; margin-bottom: 6px;"
        >
          Общая стоимость
        </div>
        <div
          :class="['num', blurClass]"
          style="font-size: 30px; font-weight: 600; letter-spacing: -0.025em; line-height: 1.05;"
        >
          {{ formatCurrency(grandTotal, ui.displayCurrency) }}
        </div>
        <div
          style="font-size: 11.5px; color: hsl(var(--muted-foreground)); margin-top: 6px;"
        >
          {{ positions.length }} позиций · {{ accountCount }} аккаунта · {{ formatDate(new Date()) }}
        </div>
      </div>

      <div
        v-for="(c, i) in summaryCards"
        :key="c.key"
        :style="{
          padding: '20px 22px',
          borderRight: i < summaryCards.length - 1 ? '1px solid hsl(var(--border))' : 'none',
        }"
      >
        <div
          class="uppercase"
          style="font-size: 10.5px; color: hsl(var(--muted-foreground)); letter-spacing: 0.08em; margin-bottom: 6px;"
        >
          {{ c.label }}
        </div>
        <div
          :class="['num', blurClass]"
          style="font-size: 20px; font-weight: 600; letter-spacing: -0.015em;"
        >
          {{ formatCompact(c.value, ui.displayCurrency) }}
        </div>
        <div class="flex items-center" style="gap: 6px; margin-top: 8px">
          <div style="flex: 1; height: 2px; background: hsl(var(--soft)); border-radius: 1px;">
            <div
              :style="{
                width: `${c.share * 100}%`,
                height: '100%',
                background: 'hsl(var(--accent))',
                borderRadius: '1px',
              }"
            />
          </div>
          <span
            class="num"
            style="font-size: 11px; color: hsl(var(--muted-foreground));"
          >{{ (c.share * 100).toFixed(1) }}%</span>
        </div>
      </div>
    </div>

    <div style="padding: 16px 22px 28px">
      <div class="flex items-center justify-between" style="margin-bottom: 10px">
        <h2 style="font-size: 12.5px; font-weight: 600; margin: 0">
          Позиции
          <span style="color: hsl(var(--muted-foreground)); font-weight: 400; margin-left: 6px;">
            {{ positions.length }}
          </span>
        </h2>
        <div
          class="flex"
          style="gap: 6px; font-size: 11px; color: hsl(var(--muted-foreground));"
        >
          <span
            style="padding: 2px 8px; border-radius: 3px; border: 1px solid hsl(var(--border)); background: hsl(var(--panel));"
          >Все классы</span>
          <span
            style="padding: 2px 8px; border-radius: 3px; border: 1px solid hsl(var(--border)); background: hsl(var(--panel));"
          >Все аккаунты</span>
        </div>
      </div>

      <div
        v-if="positions.length === 0"
        style="border: 1px solid hsl(var(--border)); border-radius: 6px; background: hsl(var(--panel)); padding: 48px 24px; text-align: center;"
      >
        <p
          style="font-size: 12.5px; color: hsl(var(--muted-foreground)); margin: 0 0 8px;"
        >Нет позиций</p>
        <p style="margin: 0; font-size: 12.5px">
          Зайди в
          <RouterLink
            to="/accounts"
            style="color: hsl(var(--accent)); text-decoration: underline;"
          >Аккаунты</RouterLink>
          и добавь первую.
        </p>
      </div>

      <div
        v-else
        style="border: 1px solid hsl(var(--border)); border-radius: 6px; overflow: hidden; background: hsl(var(--panel));"
      >
        <table style="width: 100%; border-collapse: collapse; font-size: 12px">
          <thead>
            <tr style="background: hsl(var(--background))">
              <th
                v-for="(h, i) in ['Тикер','Класс','Аккаунт','Кол-во','Цена','Обновлена','Стоимость','Доля']"
                :key="h"
                class="uppercase"
                :style="{
                  textAlign: i >= 3 ? 'right' : 'left',
                  padding: '7px 12px',
                  fontSize: '10.5px',
                  fontWeight: 500,
                  color: 'hsl(var(--muted-foreground))',
                  letterSpacing: '0.05em',
                  borderBottom: '1px solid hsl(var(--border))',
                }"
              >{{ h }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(p, i) in positions"
              :key="`${p.accountId}/${p.instrumentId}`"
              :style="{ borderTop: i ? '1px solid hsl(var(--border))' : 'none' }"
            >
              <td
                class="num"
                style="padding: 6px 12px; font-weight: 600; font-size: 11.5px"
              >{{ p.ticker }}</td>
              <td style="padding: 6px 12px">
                <span
                  class="num uppercase"
                  :style="{
                    fontSize: '10px',
                    padding: '2px 7px',
                    borderRadius: '3px',
                    background: CLASS_BADGE[p.assetClass]?.tint ?? 'hsl(var(--soft))',
                    color: 'hsl(var(--foreground))',
                    letterSpacing: '0.05em',
                  }"
                >{{ CLASS_BADGE[p.assetClass]?.label ?? p.assetClass }}</span>
              </td>
              <td
                style="padding: 6px 12px; color: hsl(var(--muted-foreground)); font-size: 11.5px;"
              >{{ p.accountName }}</td>
              <td
                class="num"
                style="padding: 6px 12px; text-align: right; font-size: 11.5px;"
              >{{ formatQuantity(p.quantity) }}</td>
              <td
                class="num"
                style="padding: 6px 12px; text-align: right; color: hsl(var(--muted-foreground)); font-size: 11.5px;"
              >
                <span v-if="p.price">{{ formatNumber(p.price, 2) }}</span>
                <span v-else style="opacity: 0.5">—</span>
              </td>
              <td
                class="num"
                :style="{
                  padding: '6px 12px',
                  textAlign: 'right',
                  fontSize: '11px',
                  ...priceAgeStyle(p.priceFetchedAt, p.priceStale),
                }"
              >
                {{ p.priceFetchedAt ? formatRelative(p.priceFetchedAt) : "—" }}
              </td>
              <td
                :class="['num', blurClass]"
                style="padding: 6px 12px; text-align: right; font-weight: 500; font-size: 12px;"
              >
                <span v-if="p.valueDisplay">
                  {{ formatNumber(p.valueDisplay, 0) }} {{ valueDisplaySuffix() }}
                </span>
                <span v-else style="opacity: 0.5">—</span>
              </td>
              <td style="padding: 6px 12px; text-align: right">
                <div class="inline-flex items-center justify-end" style="gap: 6px">
                  <div
                    style="width: 40px; height: 3px; background: hsl(var(--soft)); border-radius: 2px;"
                  >
                    <div
                      :style="{
                        width: `${Math.min(100, shareOf(p.valueDisplay) * 4 * 100)}%`,
                        height: '100%',
                        background: 'hsl(var(--accent))',
                        opacity: 0.75,
                        borderRadius: '2px',
                      }"
                    />
                  </div>
                  <span
                    class="num"
                    style="color: hsl(var(--muted-foreground)); font-size: 10.5px; min-width: 32px; text-align: right;"
                  >{{ (shareOf(p.valueDisplay) * 100).toFixed(1) }}%</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </template>
</template>
