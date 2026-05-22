<script setup lang="ts">
import { ref, computed } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useListInstruments,
  useDeleteInstrument,
  getListInstrumentsQueryKey,
} from "@/api/generated";
import type { Instrument } from "@/api/generated/model/instrument";
import { HttpError } from "@/api/mutator";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import { ASSET_CLASS_LABELS } from "@/lib/assetClass";
import { formatNumber, formatRelative } from "@/lib/formatters";
import { confirm } from "@/lib/confirm";
import { Plus, Pencil, CircleDollarSign, Trash2 } from "lucide-vue-next";
import { toast } from "vue-sonner";
import AssetFormDialog from "../components/AssetFormDialog.vue";
import AssetPriceEditDialog from "../components/AssetPriceEditDialog.vue";

const queryClient = useQueryClient();
const deleteMutation = useDeleteInstrument();

const showForm = ref(false);
const editingInstrument = ref<Instrument | null>(null);
const showPriceEdit = ref(false);
const pricingInstrument = ref<Instrument | null>(null);

const params = computed(() => ({ scope: "mine" as const, limit: 100, offset: 0 }));
const query = useListInstruments(params);
const items = computed(() => query.data.value?.items ?? []);

function openCreate() {
  editingInstrument.value = null;
  showForm.value = true;
}

function openEdit(asset: Instrument) {
  editingInstrument.value = asset;
  showForm.value = true;
}

function openPriceEdit(asset: Instrument) {
  pricingInstrument.value = asset;
  showPriceEdit.value = true;
}

async function deleteAsset(asset: Instrument) {
  const ok = await confirm({
    title: `Удалить «${asset.name}»?`,
    body: "Это действие нельзя отменить.",
    confirmText: "Удалить",
    danger: true,
  });
  if (!ok) return;
  try {
    await deleteMutation.mutateAsync({ instrumentId: asset.id });
    queryClient.invalidateQueries({ queryKey: getListInstrumentsQueryKey() });
    queryClient.invalidateQueries({ queryKey: ["portfolio"] });
  } catch (e) {
    if (e instanceof HttpError && e.status === 409) {
      toast.error("Сначала удали позиции с этим активом");
    } else {
      toast.error("Не удалось удалить: " + (e as Error).message);
    }
  }
}
</script>

<template>
  <div class="space-y-4 p-4 sm:p-6">
    <div class="flex items-center justify-between gap-4">
      <h1 class="text-xl sm:text-2xl font-semibold">Имущество</h1>
      <Button size="sm" @click="openCreate">
        <Plus :size="14" class="mr-1.5" />
        Добавить актив
      </Button>
    </div>

    <p v-if="query.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="query.isError.value" class="text-sm text-red-600">
      Не удалось загрузить
    </p>

    <div
      v-else-if="!items.length"
      class="border border-border rounded-md bg-panel px-6 py-12 text-center space-y-3"
    >
      <p class="font-medium">Нет личных активов</p>
      <p class="text-sm text-muted-foreground max-w-sm mx-auto">
        Здесь хранятся личные активы — недвижимость, транспорт и прочее имущество.
        Добавь актив и веди учёт его стоимости вручную.
      </p>
      <Button size="sm" @click="openCreate">
        <Plus :size="14" class="mr-1.5" />
        Добавить первый актив
      </Button>
    </div>

    <div v-else class="border border-border rounded-md overflow-hidden">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Название</TableHead>
            <TableHead>Тип</TableHead>
            <TableHead>Валюта</TableHead>
            <TableHead class="text-right">Цена</TableHead>
            <TableHead>Обновлено</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="asset in items" :key="asset.id">
            <TableCell class="font-medium">{{ asset.name }}</TableCell>
            <TableCell>
              <span class="num uppercase text-xs px-2 py-0.5 rounded bg-soft tracking-wider">
                {{ ASSET_CLASS_LABELS[asset.assetClass] ?? asset.assetClass }}
              </span>
            </TableCell>
            <TableCell class="num">{{ asset.currency }}</TableCell>
            <TableCell class="num text-right">
              <span v-if="asset.currentPrice">
                {{ formatNumber(asset.currentPrice, 2) }}
              </span>
              <span v-else class="text-muted-foreground opacity-50">—</span>
            </TableCell>
            <TableCell class="text-sm text-muted-foreground">
              {{ formatRelative(asset.priceUpdatedAt) }}
            </TableCell>
            <TableCell>
              <div class="flex items-center justify-end gap-1">
                <button
                  type="button"
                  title="Обновить цену"
                  class="p-1.5 rounded text-muted-foreground hover:bg-soft hover:text-foreground transition-colors"
                  @click="openPriceEdit(asset)"
                >
                  <CircleDollarSign :size="14" />
                </button>
                <button
                  type="button"
                  title="Редактировать"
                  class="p-1.5 rounded text-muted-foreground hover:bg-soft hover:text-foreground transition-colors"
                  @click="openEdit(asset)"
                >
                  <Pencil :size="14" />
                </button>
                <button
                  type="button"
                  title="Удалить"
                  class="p-1.5 rounded text-red-400 hover:bg-soft hover:text-red-600 transition-colors"
                  @click="deleteAsset(asset)"
                >
                  <Trash2 :size="14" />
                </button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>

  <AssetFormDialog
    :open="showForm"
    :instrument="editingInstrument ?? undefined"
    @update:open="showForm = $event"
  />

  <AssetPriceEditDialog
    v-if="pricingInstrument"
    :open="showPriceEdit"
    :instrument="pricingInstrument"
    @update:open="showPriceEdit = $event"
  />
</template>
