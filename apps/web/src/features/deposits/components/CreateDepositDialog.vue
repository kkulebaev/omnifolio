<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useCreateDeposit,
  getListDepositsQueryKey,
} from "@/api/generated";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUiStore } from "@/stores/ui";

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const ui = useUiStore();
const queryClient = useQueryClient();
const createMutation = useCreateDeposit();

const now = new Date();
const month = ref<number>(now.getMonth() + 1);
const year = ref<number>(now.getFullYear());
const amount = ref<string>("");
const error = ref<string | null>(null);

const monthLabels = [
  "Январь",
  "Февраль",
  "Март",
  "Апрель",
  "Май",
  "Июнь",
  "Июль",
  "Август",
  "Сентябрь",
  "Октябрь",
  "Ноябрь",
  "Декабрь",
];

const years = computed<number[]>(() => {
  const cur = new Date().getFullYear();
  const list: number[] = [];
  for (let y = cur + 1; y >= cur - 15; y--) list.push(y);
  return list;
});

watch(
  () => props.open,
  (v) => {
    if (v) {
      const fresh = new Date();
      month.value = fresh.getMonth() + 1;
      year.value = fresh.getFullYear();
      amount.value =
        ui.defaultDepositAmount != null ? String(ui.defaultDepositAmount) : "";
      error.value = null;
    }
  },
);

async function submit() {
  error.value = null;
  const trimmed = amount.value.trim();
  if (!/^[1-9][0-9]*$/.test(trimmed)) {
    error.value = "Сумма должна быть целым положительным числом";
    return;
  }
  const m = String(month.value).padStart(2, "0");
  try {
    await createMutation.mutateAsync({
      data: {
        month: `${year.value}-${m}-01`,
        amount: trimmed,
      },
    });
    queryClient.invalidateQueries({ queryKey: getListDepositsQueryKey() });
    emit("update:open", false);
  } catch (e) {
    if (e instanceof HttpError && e.status === 422) {
      error.value = e.problem.title || "Validation failed";
    } else {
      error.value = "Не удалось создать: " + (e as Error).message;
    }
  }
}

const selectClass =
  "flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50";
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">Новое пополнение</h2>

    <form class="space-y-4" @submit.prevent="submit">
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-1.5">
          <Label for="dep-month">Месяц</Label>
          <select
            id="dep-month"
            v-model.number="month"
            :class="selectClass"
          >
            <option v-for="(label, i) in monthLabels" :key="i" :value="i + 1">
              {{ label }}
            </option>
          </select>
        </div>
        <div class="space-y-1.5">
          <Label for="dep-year">Год</Label>
          <select
            id="dep-year"
            v-model.number="year"
            :class="selectClass"
          >
            <option v-for="y in years" :key="y" :value="y">{{ y }}</option>
          </select>
        </div>
      </div>

      <div class="space-y-1.5">
        <Label for="dep-amount">Сумма, ₽</Label>
        <Input
          id="dep-amount"
          v-model="amount"
          type="text"
          inputmode="numeric"
          placeholder="50000"
          autocomplete="off"
        />
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
        <Button type="submit" :disabled="createMutation.isPending.value">
          {{ createMutation.isPending.value ? "Создаём…" : "Создать" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
