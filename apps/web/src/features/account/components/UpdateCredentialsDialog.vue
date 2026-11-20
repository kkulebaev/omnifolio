<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useUpdateAccountCredentials,
  getGetAccountQueryKey,
  getListAccountsQueryKey,
} from "@/api/generated";
import type { AccountType } from "@/api/generated/model/accountType";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const props = defineProps<{
  open: boolean;
  accountId: string;
  accountType: AccountType;
}>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const queryClient = useQueryClient();
const mutation = useUpdateAccountCredentials();

const token = ref("");
const apiKey = ref("");
const apiSecret = ref("");
const error = ref<string | null>(null);

const isTInvest = computed(() => props.accountType === "tinvest");
const sourceName = computed(() => {
  switch (props.accountType) {
    case "tinvest":
      return "T-Invest";
    case "bybit":
      return "Bybit";
    case "binance":
      return "Binance";
    default:
      return props.accountType;
  }
});

watch(
  () => props.open,
  (v) => {
    if (!v) {
      token.value = "";
      apiKey.value = "";
      apiSecret.value = "";
      error.value = null;
    }
  },
);

async function submit() {
  error.value = null;
  if (isTInvest.value) {
    if (token.value.length < 10) {
      error.value = "Токен слишком короткий";
      return;
    }
  } else if (apiKey.value.length < 8 || apiSecret.value.length < 8) {
    error.value = "API key и secret обязательны";
    return;
  }

  try {
    await mutation.mutateAsync({
      accountId: props.accountId,
      data: isTInvest.value
        ? { token: token.value }
        : { apiKey: apiKey.value, apiSecret: apiSecret.value },
    });
    queryClient.invalidateQueries({ queryKey: getGetAccountQueryKey(props.accountId) });
    queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
    emit("update:open", false);
  } catch (e) {
    if (e instanceof HttpError && e.status === 422) {
      error.value = e.problem.fields?.token || e.problem.fields?.apiKey
        ? `Ключ отклонён ${sourceName.value}. Проверь правильность и права.`
        : e.problem.title || "Validation failed";
    } else {
      error.value = "Не удалось сохранить: " + (e as Error).message;
    }
  }
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">Новый ключ {{ sourceName }}</h2>

    <form class="space-y-4" @submit.prevent="submit">
      <div v-if="isTInvest" class="space-y-1.5">
        <Label for="cred-token">T-Invest токен</Label>
        <Input id="cred-token" v-model="token" type="password" placeholder="t.xxxxx..." autocomplete="off" />
        <p class="text-xs opacity-60">Счёт T-Invest останется тем же — меняется только токен.</p>
      </div>
      <template v-else>
        <div class="space-y-1.5">
          <Label for="cred-key">API Key</Label>
          <Input id="cred-key" v-model="apiKey" autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label for="cred-secret">API Secret</Label>
          <Input id="cred-secret" v-model="apiSecret" type="password" autocomplete="off" />
        </div>
      </template>
      <p class="text-xs opacity-60">
        Ключ проверяется у {{ sourceName }} перед сохранением, после — сразу запускается синхронизация.
      </p>

      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Отмена</Button>
        <Button type="submit" :disabled="mutation.isPending.value">
          {{ mutation.isPending.value ? "Проверяю…" : "Сохранить" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
