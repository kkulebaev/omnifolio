<script setup lang="ts">
import { ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  searchInstruments,
  useCreatePosition,
  getGetAccountQueryKey,
} from "@/api/generated";
import type { Instrument } from "@/api/generated/model/instrument";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import AssetFormDialog from "@/features/asset/components/AssetFormDialog.vue";
import { useAuthStore } from "@/stores/auth";

const props = defineProps<{ open: boolean; accountId: string }>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const queryClient = useQueryClient();
const createPosition = useCreatePosition();
const auth = useAuthStore();

const query = ref("");
const results = ref<Instrument[]>([]);
const selected = ref<Instrument | null>(null);
const quantity = ref("");
const error = ref<string | null>(null);

const showAssetCreate = ref(false);

const assetCreatePrefill = ref<{
  name?: string;
  ticker?: string;
  assetClass?: "real_estate" | "vehicle" | "other_asset";
  currency?: string;
}>({});

function openCreateAsset() {
  const q = query.value.trim();
  assetCreatePrefill.value = {
    name: q,
    ticker: q.toUpperCase().replace(/[^A-Za-z0-9_-]/g, "_").slice(0, 32) || "ASSET",
    assetClass: "other_asset",
    currency: auth.user?.displayCurrency ?? "RUB",
  };
  showAssetCreate.value = true;
}

function onAssetCreated(instrument: Instrument) {
  selected.value = instrument;
  results.value = [instrument];
  showAssetCreate.value = false;
}

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
      showAssetCreate.value = false;
    }
  },
);

async function save() {
  error.value = null;
  if (!selected.value) {
    error.value = "Выбери инструмент из каталога";
    return;
  }
  try {
    await createPosition.mutateAsync({
      accountId: props.accountId,
      data: { instrumentId: selected.value.id, quantity: quantity.value },
    });
    queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(props.accountId) });
    queryClient.invalidateQueries({ queryKey: ["portfolio"] });
    emit("update:open", false);
  } catch (e) {
    error.value = "Не удалось добавить позицию: " + (e as Error).message;
  }
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">Добавить позицию</h2>

    <form class="space-y-4" @submit.prevent="save">
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
      <div v-else-if="query" class="space-y-2">
        <p class="text-sm opacity-60">Ничего не найдено в каталоге.</p>
        <button
          type="button"
          class="text-sm underline text-muted-foreground hover:text-foreground"
          @click="openCreateAsset"
        >
          Создать новый актив «{{ query }}»
        </button>
      </div>

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

  <AssetFormDialog
    :open="showAssetCreate"
    :prefill="assetCreatePrefill"
    @update:open="showAssetCreate = $event"
    @created="onAssetCreated"
  />
</template>
