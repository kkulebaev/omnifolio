<script setup lang="ts">
import { computed, ref } from "vue";
import {
  VisXYContainer,
  VisLine,
  VisCrosshair,
  VisTooltip,
  VisArea,
} from "@unovis/vue";
import { useGetPortfolioHistory } from "@/api/generated";
import { formatCompact, formatDate } from "@/lib/formatters";

function niceTicks(min: number, max: number, count: number): number[] {
  if (
    !Number.isFinite(min) ||
    !Number.isFinite(max) ||
    min >= max ||
    count < 2
  ) {
    return Number.isFinite(min) ? [min] : [];
  }
  const rough = (max - min) / (count - 1);
  const mag = Math.pow(10, Math.floor(Math.log10(rough)));
  const norm = rough / mag;
  let nice: number;
  if (norm < 1.5) nice = 1;
  else if (norm < 3) nice = 2;
  else if (norm < 7) nice = 5;
  else nice = 10;
  const step = nice * mag;
  const start = Math.ceil(min / step) * step;
  const ticks: number[] = [];
  for (let v = start; v <= max + step * 1e-6; v += step) {
    ticks.push(Number(v.toFixed(10)));
  }
  return ticks;
}

const xTickFmt = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "short",
});

function formatXTick(ts: number): string {
  return xTickFmt.format(new Date(ts));
}

function makeFormatYTick(range: number): (v: number) => string {
  return (v) => {
    const abs = Math.abs(v);
    if (abs >= 1_000_000) {
      const decimals = range < 100_000 ? 3 : range < 1_000_000 ? 2 : 1;
      return `${(v / 1_000_000).toFixed(decimals)}М`;
    }
    if (abs >= 1_000) {
      const decimals = range < 1_000 ? 2 : range < 10_000 ? 1 : 0;
      return `${(v / 1_000).toFixed(decimals)}К`;
    }
    return `${Math.round(v)}`;
  };
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

const yDomain = computed<[number, number] | undefined>(() => {
  const pts = points.value;
  if (pts.length === 0) return undefined;
  let min = Infinity;
  let max = -Infinity;
  for (const p of pts) {
    if (p.total < min) min = p.total;
    if (p.total > max) max = p.total;
  }
  if (!Number.isFinite(min) || !Number.isFinite(max)) return undefined;
  const span = max - min;
  const padding =
    span > 0 ? span * 0.08 : Math.max(Math.abs(max) * 0.01, 1);
  return [min - padding, max + padding];
});

const yBaseline = computed(() => yDomain.value?.[0] ?? 0);

const formatYTick = computed(() => {
  const d = yDomain.value;
  return makeFormatYTick(d ? d[1] - d[0] : 0);
});

const X_LABEL_RESERVE_PCT = 18;

const yTicks = computed<number[]>(() => {
  const d = yDomain.value;
  if (!d) return [];
  const span = d[1] - d[0];
  if (span <= 0) return [];
  const cutoff = d[0] + span * (X_LABEL_RESERVE_PCT / 100);
  return niceTicks(d[0], d[1], 4)
    .filter((t) => t > cutoff && t < d[1])
    .reverse();
});

const xTicks = computed<number[]>(() => {
  const pts = points.value;
  if (pts.length === 0) return [];
  const first = pts[0].ts;
  const last = pts[pts.length - 1].ts;
  if (pts.length === 1 || first === last) return [first];
  if (pts.length === 2) return [first, last];
  return [first, (first + last) / 2, last];
});

function yPct(v: number): number {
  const d = yDomain.value;
  if (!d || d[1] === d[0]) return 50;
  return (1 - (v - d[0]) / (d[1] - d[0])) * 100;
}

function xPct(ts: number): number {
  const pts = points.value;
  if (pts.length === 0) return 0;
  const first = pts[0].ts;
  const last = pts[pts.length - 1].ts;
  if (first === last) return 50;
  return ((ts - first) / (last - first)) * 100;
}

function yLabelStyle(i: number, n: number, t: number): Record<string, string> {
  const pct = yPct(t);
  let transform = "translateY(-50%)";
  if (i === 0) transform = "translateY(0)";
  else if (i === n - 1) transform = "translateY(-100%)";
  return { top: `${pct}%`, transform };
}

function xLabelStyle(i: number, n: number, ts: number): Record<string, string> {
  const pct = xPct(ts);
  let transform = "translateX(-50%)";
  if (i === 0) transform = "translateX(0)";
  else if (i === n - 1) transform = "translateX(-100%)";
  return { left: `${pct}%`, transform };
}

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
    <div
      v-else
      class="relative rounded-lg border border-border bg-panel overflow-hidden"
    >
      <VisXYContainer
        class="md:hidden"
        :data="points"
        :height="180"
        :margin="{ top: 0, right: 0, bottom: 0, left: 0 }"
        :y-domain="yDomain"
      >
        <VisArea
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          :baseline="yBaseline"
          color="var(--color-accent)"
          :opacity="0.15"
        />
        <VisLine
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
        />
        <VisCrosshair :template="tooltipTemplate" />
        <VisTooltip />
      </VisXYContainer>
      <VisXYContainer
        class="hidden md:block"
        :data="points"
        :height="220"
        :margin="{ top: 0, right: 0, bottom: 0, left: 0 }"
        :y-domain="yDomain"
      >
        <VisArea
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          :baseline="yBaseline"
          color="var(--color-accent)"
          :opacity="0.15"
        />
        <VisLine
          :x="(d: Point) => d.ts"
          :y="(d: Point) => d.total"
          color="var(--color-accent)"
        />
        <VisCrosshair :template="tooltipTemplate" />
        <VisTooltip />
      </VisXYContainer>

      <div
        class="absolute inset-0 pointer-events-none text-xs text-muted-foreground"
      >
        <span
          v-for="(t, i) in yTicks"
          :key="`y-${t}`"
          :style="yLabelStyle(i, yTicks.length, t)"
          class="num absolute left-1 px-1 rounded-sm leading-none"
        >
          {{ formatYTick(t) }}
        </span>
        <span
          v-for="(t, i) in xTicks"
          :key="`x-${t}`"
          :style="xLabelStyle(i, xTicks.length, t)"
          class="num absolute bottom-1 px-1 rounded-sm leading-none"
        >
          {{ formatXTick(t) }}
        </span>
      </div>
    </div>
  </section>
</template>
