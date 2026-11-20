<script setup lang="ts">
import { computed } from "vue";
import { useGetPortfolioReturns } from "@/api/generated";
import { useUiStore } from "@/stores/ui";
import { HttpError } from "@/api/mutator";
import type { ReturnsUnavailableReason } from "@/api/generated/model/returnsUnavailableReason";
import { formatCurrency, formatDay, formatRatio } from "@/lib/formatters";
import { InfoHint } from "@/components/ui/info-hint";

const ui = useUiStore();

const returns = useGetPortfolioReturns(
  computed(() => ({ currency: ui.displayCurrency })),
  {
    query: {
      queryKey: computed(() => ["portfolio", "returns", ui.displayCurrency] as const),
    },
  },
);

const data = computed(() => returns.data.value);
const blurClass = computed(() => (ui.privacy ? "privacy-blur" : ""));

const errorText = computed(() => {
  const e: unknown = returns.error.value;
  return e instanceof HttpError && e.problem.detail
    ? e.problem.detail
    : "Не удалось посчитать доходность";
});

const hasFlows = computed(() => data.value?.firstFlowDate != null);

function signClass(v: string | null | undefined): string {
  const n = Number(v);
  if (v == null || !Number.isFinite(n) || n === 0) return "";
  return n > 0 ? "text-pos" : "text-neg";
}

const profitSigned = computed(() => {
  const p = Number(data.value?.profit ?? 0);
  const s = formatCurrency(Math.abs(p), data.value?.currency ?? ui.displayCurrency);
  if (p > 0) return `+${s}`;
  if (p < 0) return `−${s}`;
  return s;
});

const REASON_HINTS: Record<ReturnsUnavailableReason, string> = {
  no_flows: "Нет пополнений",
  insufficient_history: "Меньше 30 дней с первого пополнения — годовая ставка была бы шумом",
  no_sign_change: "Нужны и вложения, и ненулевая стоимость портфеля",
  no_convergence: "Расчёт не сошёлся на этих потоках",
  no_snapshots: "Пока нет ни одного снимка портфеля — первый появится после ночного запуска",
  no_measurable_period: "Ни один период между снимками нельзя измерить (нулевая стоимость)",
};

function reasonHint(r: ReturnsUnavailableReason | null | undefined): string | undefined {
  return r ? REASON_HINTS[r] : undefined;
}

const twrSubtitle = computed(() => {
  const d = data.value;
  if (!d?.twrFrom) return "";
  const since = `с ${formatDay(d.twrFrom)}`;
  return d.twrAnnualized != null
    ? `${formatRatio(d.twrAnnualized)} годовых · ${since}`
    : `за всё время · ${since}`;
});
</script>

<template>
  <section class="px-4 sm:px-6 pt-4 pb-5 border-b border-border">
    <h2 class="text-xs font-semibold m-0 mb-2.5">Доходность</h2>

    <div
      v-if="returns.isLoading.value"
      class="text-xs text-muted-foreground"
    >
      Загрузка…
    </div>
    <div v-else-if="returns.isError.value" class="text-xs text-neg">
      {{ errorText }}
    </div>
    <div
      v-else-if="!hasFlows"
      class="border border-dashed border-border rounded-lg px-6 py-6 text-center text-xs text-muted-foreground"
    >
      Чтобы посчитать доходность, добавь пополнения в
      <RouterLink to="/deposits" class="text-accent underline">журнал</RouterLink>.
    </div>
    <template v-else-if="data">
      <div
        class="grid grid-cols-2 lg:grid-cols-4 border border-border rounded-lg bg-panel overflow-hidden"
      >
        <div class="px-4 py-3 border-r border-b lg:border-b-0 border-border">
          <div class="flex items-center gap-1 uppercase text-xs text-muted-foreground tracking-wider mb-1">
            Вложено
            <InfoHint label="Что такое «вложено»">
              <p class="m-0">Сколько своих денег вы реально положили в портфель: все пополнения минус все выводы.</p>
              <p class="m-0 text-muted-foreground">Каждая операция пересчитана в валюту отображения по курсу ЦБ на её дату.</p>
            </InfoHint>
          </div>
          <div :class="['num', blurClass, 'text-base sm:text-lg font-semibold tracking-tight']">
            {{ formatCurrency(data.netInvested, data.currency) }}
          </div>
          <div v-if="data.firstFlowDate" class="text-xs text-muted-foreground mt-1">
            пополнения − выводы, с {{ formatDay(data.firstFlowDate) }}
          </div>
        </div>

        <div class="px-4 py-3 border-b lg:border-b-0 lg:border-r border-border">
          <div class="flex items-center gap-1 uppercase text-xs text-muted-foreground tracking-wider mb-1">
            Прибыль
            <InfoHint label="Что такое «прибыль»">
              <p class="m-0">Текущая стоимость портфеля минус вложенное. Процент — прибыль относительно вложенного, без учёта срока.</p>
            </InfoHint>
          </div>
          <div
            :class="['num', blurClass, 'text-base sm:text-lg font-semibold tracking-tight', signClass(data.profit)]"
          >
            {{ profitSigned }}
          </div>
          <div
            class="num text-xs mt-1"
            :class="data.profitPct != null ? signClass(data.profitPct) : 'text-muted-foreground'"
            :title="data.profitPct == null ? 'Нетто-вложения не положительны — процент не определён' : undefined"
          >
            {{ formatRatio(data.profitPct) }} от вложенного
          </div>
        </div>

        <div class="px-4 py-3 border-r border-border">
          <div class="flex items-center gap-1 uppercase text-xs text-muted-foreground tracking-wider mb-1">
            Ваша доходность
            <InfoHint label="Что такое XIRR">
              <p class="m-0"><b>XIRR</b> — под какой процент годовых как будто лежали ваши деньги с учётом того, когда и сколько вы вносили и выводили.</p>
              <p class="m-0">Можно сравнивать со ставкой по вкладу: «вклад под X% дал бы тот же результат».</p>
              <p class="m-0 text-muted-foreground">Считается, когда с первого пополнения прошло хотя бы 30 дней.</p>
            </InfoHint>
          </div>
          <div
            class="num text-base sm:text-lg font-semibold tracking-tight"
            :class="data.xirr != null ? signClass(data.xirr) : 'text-muted-foreground'"
            :title="data.xirr == null ? reasonHint(data.xirrReason) : undefined"
          >
            {{ formatRatio(data.xirr) }}
          </div>
          <div class="text-xs text-muted-foreground mt-1">годовых · XIRR</div>
        </div>

        <div class="px-4 py-3">
          <div class="flex items-center gap-1 uppercase text-xs text-muted-foreground tracking-wider mb-1">
            Рост активов
            <InfoHint label="Что такое TWR">
              <p class="m-0"><b>TWR</b> — насколько выросли сами активы, без влияния пополнений и выводов. Показывает качество выбора активов, а не удачу с моментом взноса.</p>
              <p class="m-0">Подходит для сравнения с индексом или другим портфелем.</p>
              <p class="m-0 text-muted-foreground">Если «ваша доходность» ниже — крупные пополнения пришлись на неудачные моменты; если выше — на удачные.</p>
            </InfoHint>
          </div>
          <div
            class="num text-base sm:text-lg font-semibold tracking-tight"
            :class="data.twr != null ? signClass(data.twr) : 'text-muted-foreground'"
            :title="data.twr == null ? reasonHint(data.twrReason) : undefined"
          >
            {{ formatRatio(data.twr) }}
          </div>
          <div class="text-xs text-muted-foreground mt-1">
            {{ twrSubtitle || "за период" }} · TWR
          </div>
        </div>
      </div>
      <p v-if="data.fxApproximated" class="text-xs text-muted-foreground mt-1.5 mb-0">
        * Часть сумм пересчитана по ближайшему доступному курсу ЦБ — исторического
        на дату операции нет.
      </p>
    </template>
  </section>
</template>
