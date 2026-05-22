<script setup lang="ts">
import { ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import { useSetInstrumentPrice, getListInstrumentsQueryKey } from "@/api/generated";
import type { Instrument } from "@/api/generated/model/instrument";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "vue-sonner";

const props = defineProps<{
  open: boolean;
  instrument: Instrument;
}>();

const emit = defineEmits<{ "update:open": [v: boolean] }>();

const queryClient = useQueryClient();
const setPrice = useSetInstrumentPrice();

const price = ref("");
const priceError = ref<string | null>(null);

watch(
  () => props.open,
  (v) => {
    if (v) {
      price.value = props.instrument.currentPrice ?? "";
      priceError.value = null;
    }
  },
  { immediate: true },
);

async function submit() {
  priceError.value = null;
  const numeric = Number(price.value.replace(",", "."));
  if (!price.value || isNaN(numeric) || numeric <= 0) {
    priceError.value = "Укажи положительное число";
    return;
  }

  try {
    await setPrice.mutateAsync({
      instrumentId: props.instrument.id,
      data: { price: numeric.toString() },
    });
    queryClient.invalidateQueries({ queryKey: getListInstrumentsQueryKey() });
    queryClient.invalidateQueries({ queryKey: ["portfolio"] });
    toast.success("Цена обновлена");
    emit("update:open", false);
  } catch (e) {
    if (e instanceof HttpError) {
      priceError.value = `Ошибка ${e.status}: ${e.message}`;
    } else {
      priceError.value = "Не удалось обновить цену";
    }
  }
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-1">Обновить цену</h2>
    <p class="text-sm text-muted-foreground mb-4">{{ props.instrument.name }}</p>

    <form class="space-y-4" @submit.prevent="submit">
      <div class="space-y-1.5">
        <Label for="asset-price-input">Текущая цена ({{ props.instrument.currency }})</Label>
        <Input
          id="asset-price-input"
          v-model="price"
          inputmode="decimal"
          placeholder="12000000"
          :class="priceError ? 'border-red-500' : ''"
          autofocus
        />
        <p v-if="priceError" class="text-xs text-red-600">{{ priceError }}</p>
      </div>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">
          Отмена
        </Button>
        <Button type="submit" :disabled="setPrice.isPending.value">
          {{ setPrice.isPending.value ? "Сохраняем…" : "Сохранить" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
