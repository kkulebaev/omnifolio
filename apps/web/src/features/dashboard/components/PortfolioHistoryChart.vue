<script setup lang="ts">
import { computed, ref } from "vue";
import {
  VisXYContainer,
  VisLine,
  VisAxis,
  VisCrosshair,
  VisTooltip,
  VisArea,
} from "@unovis/vue";
import { useGetPortfolioHistory } from "@/api/generated";
import { formatCompact, formatDate } from "@/lib/formatters";

const xTickFmt = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "short",
});

function formatXTick(ts: number): string {
  return xTickFmt.format(new Date(ts));
}

function formatYTick(v: number): string {
  const abs = Math.abs(v);
  if (abs >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}М`;
  if (abs >= 1_000) return `${Math.round(v / 1_000)}К`;
  return `${Math.round(v)}`;
}

type Preset = "7d" | "30d" | "90d" | "1y" | "all";

type Point = {
  ts: number;
  total: number;
  date: string;
  displayCurrency: string;
};

const preset = ref<Preset>("90d");

const presets: { key: Preset; label: string }[] = [
  { key: "7d", label: "7д" },
  { key: "30d", label: "30д" },
  { key: "90d", label: "90д" },
  { key: "1y", label: "1г" },
  { key: "all", label: "всё" },
];

function isoDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function rangeFor(p: Preset): { from?: string; to?: string } {
  const now = new Date();
  const today = new Date(
    Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()),
  );
  const days: Record<Preset, number | null> = {
    "7d": 7,
    "30d": 30,
    "90d": 90,
    "1y": 365,
    all: null,
  };
  const span = days[p];
  if (span === null) {
    return { from: "1970-01-01", to: isoDate(today) };
  }
  const from = new Date(today);
  from.setUTCDate(from.getUTCDate() - span);
  return { from: isoDate(from), to: isoDate(today) };
}

const params = computed(() => rangeFor(preset.value));

const history = useGetPortfolioHistory(params, {
  query: {
    queryKey: computed(
      () => ["portfolio-history", preset.value] as const,
    ),
  },
});

const points = computed<Point[]>(() => {
  const raw = history.data.value?.points ?? [];
  return raw.map((p) => ({
    ts: Date.parse(p.date),
    total: Number(p.grandTotal),
    date: p.date,
    displayCurrency: p.displayCurrency,
  }));
});

const isEmpty = computed(
  () => !history.isLoading.value && points.value.length === 0,
);

function tooltipTemplate(d: Point): string {
  const value = formatCompact(d.total, d.displayCurrency);
  const dateLabel = formatDate(d.date);
  return `<div style="font-size:11px;line-height:1.4">
    <div style="color:var(--color-muted-foreground)">${dateLabel}</div>
    <div style="font-weight:600">${value}</div>
  </div>`;
}
</script>

<template>
  <section class="px-4 sm:px-6 pt-4 pb-6 border-b border-border">
    <div class="flex items-center justify-between mb-2.5 gap-2 flex-wrap">
      <h2 class="text-xs font-semibold m-0">
        Динамика стоимости
        <span
          v-if="
            history.data.value &&
            history.data.value.currentDisplayCurrency !==
              points[points.length - 1]?.displayCurrency
          "
          class="text-muted-foreground font-normal ml-1.5"
        >
          (валюта менялась)
        </span>
      </h2>
      <div class="flex gap-1.5 text-xs flex-wrap">
        <button
          v-for="p in presets"
          :key="p.key"
          type="button"
          @click="preset = p.key"
          class="px-2 py-0.5 rounded-sm border border-border bg-panel cursor-pointer transition-colors outline-none hover:bg-soft"
          :class="
            preset === p.key
              ? 'text-foreground bg-soft'
              : 'text-muted-foreground'
          "
        >
          {{ p.label }}
        </button>
      </div>
    </div>

    <div
      v-if="history.isLoading.value"
      class="h-50 flex items-center justify-center text-xs text-muted-foreground"
    >
      Загрузка…
    </div>
    <div
      v-else-if="history.isError.value"
      class="h-50 flex items-center justify-center text-xs text-neg"
    >
      Не удалось загрузить историю
    </div>
    <div
      v-else-if="isEmpty"
      class="h-50 flex items-center justify-center text-xs text-muted-foreground border border-dashed border-border rounded-lg"
    >
      Снимков пока нет — первая точка появится после ближайшего ночного запуска
    </div>
    <div v-else class="rounded-lg border border-border bg-panel overflow-hidden">
      <VisXYContainer
        class="md:hidden"
        :data="points"
        :height="160"
        :margin="{ top: 8, right: 12, bottom: 20, left: 40 }"
      >
        <VisArea
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
          :opacity="0.15"
        />
        <VisLine
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
        />
        <VisAxis type="x" :tick-format="formatXTick" :num-ticks="3" />
        <VisAxis type="y" :tick-format="formatYTick" :num-ticks="3" />
        <VisCrosshair :template="tooltipTemplate" />
        <VisTooltip />
      </VisXYContainer>
      <VisXYContainer
        class="hidden md:block"
        :data="points"
        :height="200"
        :margin="{ top: 12, right: 16, bottom: 24, left: 56 }"
      >
        <VisArea
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
          :opacity="0.15"
        />
        <VisLine
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
        />
        <VisAxis type="x" :tick-format="formatXTick" :num-ticks="5" />
        <VisAxis type="y" :tick-format="formatYTick" :num-ticks="4" />
        <VisCrosshair :template="tooltipTemplate" />
        <VisTooltip />
      </VisXYContainer>
    </div>
  </section>
</template>
