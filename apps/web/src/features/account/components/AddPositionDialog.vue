<script setup lang="ts">
import { ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  searchInstruments,
  createInstrument,
  useCreatePosition,
  getGetAccountQueryKey,
} from "@/api/generated";
import { AssetClass } from "@/api/generated/model/assetClass";
import type { Instrument } from "@/api/generated/model/instrument";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const props = defineProps<{ open: boolean; accountId: string }>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const queryClient = useQueryClient();
const createPosition = useCreatePosition();

const tab = ref<"search" | "manual">("search");
const query = ref("");
const results = ref<Instrument[]>([]);
const selected = ref<Instrument | null>(null);
const quantity = ref("");
const error = ref<string | null>(null);

const manual = ref({
  ticker: "",
  assetClass: AssetClass.us_stock as AssetClass,
  currency: "USD",
  name: "",
});

let debounce: ReturnType<typeof setTimeout> | undefined;
watch(query, (q) => {
  if (debounce) clearTimeout(debounce);
  if (!q.trim()) {
    results.value = [];
    return;
  }
  debounce = setTimeout(async () => {
    try {
      const r = await searchInstruments({ q });
      results.value = r.items;
    } catch (e) {
      console.error(e);
    }
  }, 300);
});

watch(
  () => props.open,
  (v) => {
    if (!v) {
      query.value = "";
      results.value = [];
      selected.value = null;
      quantity.value = "";
      error.value = null;
      tab.value = "search";
      manual.value = {
        ticker: "",
        assetClass: AssetClass.us_stock,
        currency: "USD",
        name: "",
      };
    }
  },
);

async function save() {
  error.value = null;
  let instrumentId: string;

  if (tab.value === "search") {
    if (!selected.value) {
      error.value = "Выбери инструмент или создай вручную";
      return;
    }
    instrumentId = selected.value.id;
  } else {
    try {
      const created = await createInstrument({
        ticker: manual.value.ticker.trim().toUpperCase(),
        assetClass: manual.value.assetClass,
        currency: manual.value.currency.trim().toUpperCase(),
        name: manual.value.name.trim(),
      });
      instrumentId = created.id;
    } catch (e) {
      error.value = "Не удалось создать инструмент: " + (e as Error).message;
      return;
    }
  }

  try {
    await createPosition.mutateAsync({
      accountId: props.accountId,
      data: { instrumentId, quantity: quantity.value },
    });
    queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(props.accountId) });
    queryClient.invalidateQueries({ queryKey: ["portfolio"] });
    emit("update:open", false);
  } catch (e) {
    error.value = "Не удалось добавить позицию: " + (e as Error).message;
  }
}

const ASSET_CLASSES = Object.values(AssetClass);
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">Добавить позицию</h2>

    <div class="flex gap-1 mb-4 border-b">
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'search' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="tab = 'search'"
      >
        Поиск
      </button>
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'manual' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="tab = 'manual'"
      >
        Создать вручную
      </button>
    </div>

    <form class="space-y-4" @submit.prevent="save">
      <template v-if="tab === 'search'">
        <div class="space-y-1.5">
          <Label for="search">Поиск инструмента</Label>
          <Input id="search" v-model="query" placeholder="AAPL, Apple…" />
        </div>
        <ul v-if="results.length" class="border rounded max-h-48 overflow-auto">
          <li
            v-for="r in results"
            :key="r.id"
            class="px-3 py-2 cursor-pointer text-sm hover:bg-muted/50"
            :class="selected?.id === r.id ? 'bg-muted/40' : ''"
            @click="selected = r"
          >
            <span class="font-medium">{{ r.ticker }}</span>
            <span class="opacity-60"> · {{ r.assetClass }} · {{ r.currency }} · {{ r.name }}</span>
          </li>
        </ul>
        <p v-else-if="query" class="text-sm opacity-60">
          Ничего не найдено. Перейди на «Создать вручную».
        </p>
      </template>

      <template v-else>
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="m-ticker">Тикер</Label>
            <Input id="m-ticker" v-model="manual.ticker" placeholder="AAPL" />
          </div>
          <div class="space-y-1.5">
            <Label for="m-currency">Валюта</Label>
            <Input id="m-currency" v-model="manual.currency" placeholder="USD" />
          </div>
        </div>
        <div class="space-y-1.5">
          <Label for="m-class">Класс</Label>
          <select
            id="m-class"
            v-model="manual.assetClass"
            class="w-full rounded-md border h-9 px-2 text-sm"
            style="background-color: hsl(var(--background))"
          >
            <option v-for="c in ASSET_CLASSES" :key="c" :value="c">{{ c }}</option>
          </select>
        </div>
        <div class="space-y-1.5">
          <Label for="m-name">Название</Label>
          <Input id="m-name" v-model="manual.name" placeholder="Apple Inc." />
        </div>
      </template>

      <div class="space-y-1.5">
        <Label for="qty">Количество</Label>
        <Input id="qty" v-model="quantity" placeholder="10" />
      </div>

      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Отмена</Button>
        <Button type="submit" :disabled="createPosition.isPending.value">
          {{ createPosition.isPending.value ? "Сохраняем…" : "Добавить" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
