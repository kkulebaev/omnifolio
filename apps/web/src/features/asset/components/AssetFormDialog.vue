<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useCreateInstrument,
  useUpdateInstrument,
  getListInstrumentsQueryKey,
} from "@/api/generated";
import type { Instrument } from "@/api/generated/model/instrument";
import type { CreatePersonalInstrumentRequestAssetClass } from "@/api/generated/model/createPersonalInstrumentRequestAssetClass";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ASSET_CLASS_LABELS, PERSONAL_ASSET_CLASSES } from "@/lib/assetClass";
import { useUiStore } from "@/stores/ui";

const props = defineProps<{
  open: boolean;
  instrument?: Instrument;
  prefill?: {
    name?: string;
    ticker?: string;
    assetClass?: CreatePersonalInstrumentRequestAssetClass;
    currency?: string;
  };
}>();

const emit = defineEmits<{
  "update:open": [v: boolean];
  created: [instrument: Instrument];
}>();

const ui = useUiStore();
const queryClient = useQueryClient();
const createMutation = useCreateInstrument();
const updateMutation = useUpdateInstrument();

const isEditMode = computed(() => !!props.instrument);

const name = ref("");
const ticker = ref("");
const assetClass = ref<CreatePersonalInstrumentRequestAssetClass>("other_asset");
const currency = ref("");
const initialPrice = ref("");

const nameError = ref<string | null>(null);
const tickerError = ref<string | null>(null);
const currencyError = ref<string | null>(null);
const priceError = ref<string | null>(null);
const generalError = ref<string | null>(null);

function resetForm() {
  if (props.instrument) {
    name.value = props.instrument.name;
    ticker.value = props.instrument.ticker;
    assetClass.value = props.instrument.assetClass as CreatePersonalInstrumentRequestAssetClass;
    currency.value = props.instrument.currency;
  } else {
    name.value = props.prefill?.name ?? "";
    ticker.value = props.prefill?.ticker ?? "";
    assetClass.value = props.prefill?.assetClass ?? "other_asset";
    currency.value = props.prefill?.currency ?? ui.displayCurrency;
    initialPrice.value = "";
  }
  nameError.value = null;
  tickerError.value = null;
  currencyError.value = null;
  priceError.value = null;
  generalError.value = null;
}

watch(
  () => props.open,
  (v) => {
    if (v) resetForm();
  },
  { immediate: true },
);

function validate(): boolean {
  let ok = true;
  nameError.value = null;
  tickerError.value = null;
  currencyError.value = null;
  priceError.value = null;

  if (!name.value.trim() || name.value.trim().length > 100) {
    nameError.value = "Обязательно, до 100 символов";
    ok = false;
  }
  if (!/^[A-Za-z0-9_-]{1,32}$/.test(ticker.value)) {
    tickerError.value = "Латиница, цифры, дефис/подчёркивание, до 32 знаков";
    ok = false;
  }
  if (!isEditMode.value) {
    if (!/^[A-Z]{3,5}$/.test(currency.value)) {
      currencyError.value = "3–5 заглавных латинских букв (напр. RUB)";
      ok = false;
    }
    const price = Number(initialPrice.value.replace(",", "."));
    if (!initialPrice.value || isNaN(price) || price <= 0) {
      priceError.value = "Укажи положительное число";
      ok = false;
    }
  }
  return ok;
}

const isPending = computed(
  () => createMutation.isPending.value || updateMutation.isPending.value,
);

async function submit() {
  generalError.value = null;
  if (!validate()) return;

  try {
    if (isEditMode.value && props.instrument) {
      await updateMutation.mutateAsync({
        instrumentId: props.instrument.id,
        data: { name: name.value.trim(), ticker: ticker.value },
      });
      queryClient.invalidateQueries({ queryKey: getListInstrumentsQueryKey() });
      queryClient.invalidateQueries({ queryKey: ["portfolio"] });
      emit("update:open", false);
    } else {
      const price = initialPrice.value.replace(",", ".");
      const created = await createMutation.mutateAsync({
        data: {
          name: name.value.trim(),
          ticker: ticker.value,
          assetClass: assetClass.value,
          currency: currency.value,
          initialPrice: price,
        },
      });
      queryClient.invalidateQueries({ queryKey: getListInstrumentsQueryKey() });
      queryClient.invalidateQueries({ queryKey: ["portfolio"] });
      emit("created", created);
      emit("update:open", false);
    }
  } catch (e) {
    if (e instanceof HttpError && e.status === 409) {
      tickerError.value = "Уже есть актив с таким тикером";
    } else {
      generalError.value = "Не удалось сохранить: " + (e as Error).message;
    }
  }
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">
      {{ isEditMode ? "Редактировать актив" : "Новый личный актив" }}
    </h2>

    <form class="space-y-4" @submit.prevent="submit">
      <div class="space-y-1.5">
        <Label for="asset-name">Название</Label>
        <Input
          id="asset-name"
          v-model="name"
          placeholder="Квартира на Арбате"
          :class="nameError ? 'border-red-500' : ''"
        />
        <p v-if="nameError" class="text-xs text-red-600">{{ nameError }}</p>
      </div>

      <div class="space-y-1.5">
        <Label for="asset-ticker">Тикер</Label>
        <Input
          id="asset-ticker"
          v-model="ticker"
          placeholder="ARBAT_APT"
          :class="tickerError ? 'border-red-500' : ''"
        />
        <p class="text-xs text-muted-foreground">
          Латиница, цифры, дефис/подчёркивание — не отображается обычно, нужен для уникальности.
        </p>
        <p v-if="tickerError" class="text-xs text-red-600">{{ tickerError }}</p>
      </div>

      <template v-if="!isEditMode">
        <div class="space-y-1.5">
          <Label for="asset-class">Тип актива</Label>
          <select
            id="asset-class"
            v-model="assetClass"
            class="w-full border border-border rounded-md bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent"
          >
            <option
              v-for="cls in PERSONAL_ASSET_CLASSES"
              :key="cls"
              :value="cls"
            >
              {{ ASSET_CLASS_LABELS[cls] }}
            </option>
          </select>
        </div>

        <div class="space-y-1.5">
          <Label for="asset-currency">Валюта</Label>
          <Input
            id="asset-currency"
            v-model="currency"
            placeholder="RUB"
            :class="currencyError ? 'border-red-500' : ''"
            @input="currency = (currency ?? '').toUpperCase()"
          />
          <p v-if="currencyError" class="text-xs text-red-600">{{ currencyError }}</p>
        </div>

        <div class="space-y-1.5">
          <Label for="asset-price">Начальная цена</Label>
          <Input
            id="asset-price"
            v-model="initialPrice"
            placeholder="12000000"
            inputmode="decimal"
            :class="priceError ? 'border-red-500' : ''"
          />
          <p v-if="priceError" class="text-xs text-red-600">{{ priceError }}</p>
        </div>
      </template>

      <template v-else>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1">
            <p class="text-xs text-muted-foreground">Тип</p>
            <p class="text-sm font-medium">
              {{ ASSET_CLASS_LABELS[props.instrument!.assetClass] ?? props.instrument!.assetClass }}
            </p>
          </div>
          <div class="space-y-1">
            <p class="text-xs text-muted-foreground">Валюта</p>
            <p class="text-sm font-medium num">{{ props.instrument!.currency }}</p>
          </div>
        </div>
        <p class="text-xs text-muted-foreground">
          Тип и валюту изменить нельзя — удали актив и создай заново.
        </p>
      </template>

      <p v-if="generalError" class="text-sm text-red-600">{{ generalError }}</p>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">
          Отмена
        </Button>
        <Button type="submit" :disabled="isPending">
          {{ isPending ? "Сохраняем…" : isEditMode ? "Сохранить" : "Создать" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
