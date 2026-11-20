<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import { useCreateDeposit, useUpdateDeposit } from "@/api/generated";
import type { Deposit } from "@/api/generated/model/deposit";
import { DepositKind } from "@/api/generated/model/depositKind";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUiStore } from "@/stores/ui";
import { invalidateDepositQueries } from "../invalidate";

const props = defineProps<{ open: boolean; deposit?: Deposit | null }>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const CURRENCIES = ["RUB", "USD", "EUR", "USDT", "CNY"];
const AMOUNT_RE = /^[0-9]+(\.[0-9]{1,2})?$/;

const ui = useUiStore();
const queryClient = useQueryClient();
const createMutation = useCreateDeposit();
const updateMutation = useUpdateDeposit();

const isEdit = computed(() => props.deposit != null);
const isPending = computed(
  () => createMutation.isPending.value || updateMutation.isPending.value,
);

const kind = ref<DepositKind>(DepositKind.deposit);
const date = ref("");
const amount = ref("");
const currency = ref("RUB");
const amountError = ref<string | null>(null);
const dateError = ref<string | null>(null);
const error = ref<string | null>(null);

// Keeps a legacy/foreign currency selectable when editing such a record.
const currencyOptions = computed(() =>
  CURRENCIES.includes(currency.value)
    ? CURRENCIES
    : [...CURRENCIES, currency.value],
);

function localToday(): string {
  const d = new Date();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${m}-${day}`;
}

function reset() {
  const d = props.deposit;
  if (d) {
    kind.value = d.kind;
    date.value = d.date;
    amount.value = d.amount;
    currency.value = d.currency;
  } else {
    kind.value = DepositKind.deposit;
    date.value = localToday();
    amount.value =
      ui.defaultDepositAmount != null ? String(ui.defaultDepositAmount) : "";
    currency.value = "RUB";
  }
  amountError.value = null;
  dateError.value = null;
  error.value = null;
}

watch(
  () => props.open,
  (v) => {
    if (v) reset();
  },
  { immediate: true },
);

async function submit() {
  error.value = null;
  amountError.value = null;
  dateError.value = null;

  const normalized = amount.value.trim().replace(/\s/g, "").replace(",", ".");
  if (!AMOUNT_RE.test(normalized) || Number(normalized) <= 0) {
    amountError.value = "Положительное число, не больше 2 знаков после запятой";
  }
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date.value)) {
    dateError.value = "Укажи дату";
  }
  if (amountError.value || dateError.value) return;

  const data = {
    date: date.value,
    kind: kind.value,
    currency: currency.value,
    amount: normalized,
  };
  try {
    if (props.deposit) {
      await updateMutation.mutateAsync({ depositId: props.deposit.id, data });
    } else {
      await createMutation.mutateAsync({ data });
    }
    invalidateDepositQueries(queryClient);
    emit("update:open", false);
  } catch (e) {
    if (e instanceof HttpError && e.status === 422) {
      const fields = e.problem.fields ?? {};
      if (fields.amount) amountError.value = fields.amount;
      if (fields.date) dateError.value = fields.date;
      error.value =
        fields.amount || fields.date ? null : e.problem.title || "Validation failed";
    } else {
      error.value = "Не удалось сохранить: " + (e as Error).message;
    }
  }
}

const selectClass =
  "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50";

const kindOptions: { value: DepositKind; label: string }[] = [
  { value: DepositKind.deposit, label: "Пополнение" },
  { value: DepositKind.withdrawal, label: "Вывод" },
];
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">
      {{ isEdit ? "Редактировать запись" : "Новая запись" }}
    </h2>

    <form class="space-y-4" @submit.prevent="submit">
      <div
        class="grid grid-cols-2 gap-1 rounded-md border border-border p-1"
        role="radiogroup"
        aria-label="Тип операции"
      >
        <button
          v-for="o in kindOptions"
          :key="o.value"
          type="button"
          role="radio"
          :aria-checked="kind === o.value"
          :data-kind="o.value"
          class="rounded-sm px-3 py-1.5 text-sm transition-colors cursor-pointer"
          :class="
            kind === o.value
              ? 'bg-soft text-foreground font-medium'
              : 'text-muted-foreground hover:text-foreground'
          "
          @click="kind = o.value"
        >
          {{ o.label }}
        </button>
      </div>

      <div class="space-y-1.5">
        <Label for="dep-date">Дата</Label>
        <Input
          id="dep-date"
          v-model="date"
          type="date"
          :class="dateError ? 'border-red-500' : ''"
        />
        <p v-if="dateError" class="text-xs text-red-600">{{ dateError }}</p>
      </div>

      <div class="grid grid-cols-3 gap-3">
        <div class="space-y-1.5 col-span-2">
          <Label for="dep-amount">Сумма</Label>
          <Input
            id="dep-amount"
            v-model="amount"
            type="text"
            inputmode="decimal"
            placeholder="50000"
            autocomplete="off"
            :class="amountError ? 'border-red-500' : ''"
          />
          <p v-if="amountError" class="text-xs text-red-600">{{ amountError }}</p>
        </div>
        <div class="space-y-1.5">
          <Label for="dep-currency">Валюта</Label>
          <select id="dep-currency" v-model="currency" :class="selectClass">
            <option v-for="c in currencyOptions" :key="c" :value="c">{{ c }}</option>
          </select>
        </div>
      </div>

      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          @click="emit('update:open', false)"
        >
          Отмена
        </Button>
        <Button type="submit" :disabled="isPending">
          {{ isPending ? "Сохраняем…" : isEdit ? "Сохранить" : "Создать" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
