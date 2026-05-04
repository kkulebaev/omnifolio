<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useCreateAccount,
  previewTInvestAccounts,
  getListAccountsQueryKey,
} from "@/api/generated";
import { AccountType } from "@/api/generated/model/accountType";
import type { TInvestSubAccount } from "@/api/generated/model/tInvestSubAccount";
import { HttpError } from "@/api/mutator";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ "update:open": [v: boolean] }>();

const queryClient = useQueryClient();
const createMutation = useCreateAccount();

const tab = ref<"manual" | "tinvest" | "bybit" | "binance">("manual");

// Manual tab state
const manualName = ref("");

// T-Invest tab state
const tToken = ref("");
const tSubAccounts = ref<TInvestSubAccount[]>([]);
const tSelectedId = ref<string>("");
const tName = ref("");
const tStep = ref<"token" | "select">("token");
const previewLoading = ref(false);

// Bybit tab state
const bName = ref("");
const bApiKey = ref("");
const bApiSecret = ref("");

// Binance tab state
const biName = ref("");
const biApiKey = ref("");
const biApiSecret = ref("");

const error = ref<string | null>(null);

const showSubmit = computed(
  () =>
    tab.value === "manual" ||
    (tab.value === "tinvest" && tStep.value === "select") ||
    tab.value === "bybit" ||
    tab.value === "binance",
);

watch(
  () => props.open,
  (v) => {
    if (!v) {
      tab.value = "manual";
      manualName.value = "";
      tToken.value = "";
      tSubAccounts.value = [];
      tSelectedId.value = "";
      tName.value = "";
      tStep.value = "token";
      bName.value = "";
      bApiKey.value = "";
      bApiSecret.value = "";
      biName.value = "";
      biApiKey.value = "";
      biApiSecret.value = "";
      error.value = null;
      previewLoading.value = false;
    }
  },
);

async function previewToken() {
  error.value = null;
  if (tToken.value.length < 10) {
    error.value = "Токен слишком короткий";
    return;
  }
  previewLoading.value = true;
  try {
    const res = await previewTInvestAccounts({ token: tToken.value });
    tSubAccounts.value = res.subAccounts;
    const first = tSubAccounts.value[0];
    if (tSubAccounts.value.length === 1 && first) {
      tSelectedId.value = first.id;
    }
    tStep.value = "select";
  } catch (e) {
    if (e instanceof HttpError && e.status === 422) {
      error.value = "Токен отклонён T-Invest. Проверь правильность.";
    } else {
      error.value = "Не удалось проверить токен: " + (e as Error).message;
    }
  } finally {
    previewLoading.value = false;
  }
}

async function submit() {
  error.value = null;
  try {
    if (tab.value === "manual") {
      if (manualName.value.trim().length === 0) {
        error.value = "Название обязательно";
        return;
      }
      await createMutation.mutateAsync({
        data: { name: manualName.value.trim(), type: AccountType.manual },
      });
    } else if (tab.value === "tinvest") {
      if (!tSelectedId.value) {
        error.value = "Выбери sub-аккаунт";
        return;
      }
      if (tName.value.trim().length === 0) {
        error.value = "Название обязательно";
        return;
      }
      await createMutation.mutateAsync({
        data: {
          name: tName.value.trim(),
          type: AccountType.tinvest,
          token: tToken.value,
          tinvestAccountId: tSelectedId.value,
        },
      });
    } else if (tab.value === "bybit") {
      if (bName.value.trim().length === 0) {
        error.value = "Название обязательно";
        return;
      }
      if (bApiKey.value.length < 8 || bApiSecret.value.length < 8) {
        error.value = "API key и secret обязательны";
        return;
      }
      await createMutation.mutateAsync({
        data: {
          name: bName.value.trim(),
          type: AccountType.bybit,
          apiKey: bApiKey.value,
          apiSecret: bApiSecret.value,
        },
      });
    } else {
      if (biName.value.trim().length === 0) {
        error.value = "Название обязательно";
        return;
      }
      if (biApiKey.value.length < 8 || biApiSecret.value.length < 8) {
        error.value = "API key и secret обязательны";
        return;
      }
      await createMutation.mutateAsync({
        data: {
          name: biName.value.trim(),
          type: AccountType.binance,
          apiKey: biApiKey.value,
          apiSecret: biApiSecret.value,
        },
      });
    }
    queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
    emit("update:open", false);
  } catch (e) {
    if (e instanceof HttpError && e.status === 422) {
      const fields = e.problem.fields;
      if (fields) {
        error.value = Object.entries(fields)
          .map(([k, v]) => `${k}: ${v}`)
          .join("; ");
      } else {
        error.value = e.problem.title || "Validation failed";
      }
    } else {
      error.value = "Не удалось создать: " + (e as Error).message;
    }
  }
}

function typeBadge(type: string): string {
  switch (type) {
    case "BROKER":
      return "Брокерский";
    case "IIS":
      return "ИИС";
    case "PREMIUM":
      return "Премиум";
    default:
      return type;
  }
}
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <h2 class="text-lg font-semibold mb-4">Новый аккаунт</h2>

    <div class="flex gap-1 mb-4 border-b">
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'manual' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="
          tab = 'manual';
          error = null;
        "
      >
        Manual
      </button>
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'tinvest' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="
          tab = 'tinvest';
          error = null;
        "
      >
        T-Invest
      </button>
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'bybit' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="
          tab = 'bybit';
          error = null;
        "
      >
        Bybit
      </button>
      <button
        type="button"
        class="px-3 py-2 text-sm"
        :class="tab === 'binance' ? 'border-b-2 border-primary font-medium' : 'opacity-60'"
        @click="
          tab = 'binance';
          error = null;
        "
      >
        Binance
      </button>
    </div>

    <form class="space-y-4" @submit.prevent="submit">
      <template v-if="tab === 'manual'">
        <div class="space-y-1.5">
          <Label for="acc-name">Название</Label>
          <Input id="acc-name" v-model="manualName" placeholder="Например, Бумажные" />
        </div>
        <p class="text-sm opacity-60">Тип: <code>manual</code> — позиции добавляются вручную.</p>
      </template>

      <template v-else-if="tab === 'tinvest'">
        <template v-if="tStep === 'token'">
          <div class="space-y-1.5">
            <Label for="t-token">T-Invest токен</Label>
            <Input id="t-token" v-model="tToken" type="password" placeholder="t.xxxxx..." autocomplete="off" />
            <p class="text-xs opacity-60">
              Используй <strong>read-only</strong> токен из настроек T-Invest.
              <a
                href="https://www.tbank.ru/invest/settings/api/"
                target="_blank"
                rel="noopener"
                class="underline"
                >где взять</a
              >
            </p>
          </div>
          <div class="flex justify-end gap-2">
            <Button type="button" variant="outline" @click="emit('update:open', false)">Отмена</Button>
            <Button type="button" :disabled="previewLoading" @click="previewToken">
              {{ previewLoading ? "Проверяю…" : "Далее" }}
            </Button>
          </div>
        </template>

        <template v-else>
          <div class="space-y-2">
            <Label>Выбери счёт T-Invest</Label>
            <div v-if="tSubAccounts.length === 0" class="text-sm opacity-60">
              Нет доступных счетов
            </div>
            <ul v-else class="space-y-1">
              <li v-for="sub in tSubAccounts" :key="sub.id">
                <label
                  class="flex items-center gap-3 px-3 py-2 border rounded cursor-pointer text-sm"
                  :class="tSelectedId === sub.id ? 'bg-muted/40' : ''"
                >
                  <input
                    v-model="tSelectedId"
                    type="radio"
                    :value="sub.id"
                    class="accent-primary"
                  />
                  <div class="flex-1">
                    <div class="font-medium">{{ sub.name }}</div>
                  </div>
                  <span class="text-xs opacity-60">{{ typeBadge(sub.type) }}</span>
                </label>
              </li>
            </ul>
          </div>
          <div class="space-y-1.5">
            <Label for="t-name">Имя в Omnifolio</Label>
            <Input id="t-name" v-model="tName" placeholder="T-Invest Брокерский" />
          </div>
        </template>
      </template>

      <template v-else-if="tab === 'bybit'">
        <div class="space-y-1.5">
          <Label for="b-name">Название</Label>
          <Input id="b-name" v-model="bName" placeholder="Bybit Crypto" />
        </div>
        <div class="space-y-1.5">
          <Label for="b-key">API Key</Label>
          <Input id="b-key" v-model="bApiKey" placeholder="..." autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label for="b-secret">API Secret</Label>
          <Input
            id="b-secret"
            v-model="bApiSecret"
            type="password"
            placeholder="..."
            autocomplete="off"
          />
        </div>
        <p class="text-xs opacity-60">
          Используй <strong>read-only</strong> ключ из
          <a href="https://www.bybit.com/app/user/api-management" target="_blank" rel="noopener" class="underline">
            Bybit API Management
          </a>
          с правами Wallet → Account info.
        </p>
      </template>

      <template v-else>
        <div class="space-y-1.5">
          <Label for="bi-name">Название</Label>
          <Input id="bi-name" v-model="biName" placeholder="Binance Crypto" />
        </div>
        <div class="space-y-1.5">
          <Label for="bi-key">API Key</Label>
          <Input id="bi-key" v-model="biApiKey" placeholder="..." autocomplete="off" />
        </div>
        <div class="space-y-1.5">
          <Label for="bi-secret">API Secret</Label>
          <Input
            id="bi-secret"
            v-model="biApiSecret"
            type="password"
            placeholder="..."
            autocomplete="off"
          />
        </div>
        <p class="text-xs opacity-60">
          Используй <strong>read-only</strong> ключ из
          <a
            href="https://www.binance.com/en/my/settings/api-management"
            target="_blank"
            rel="noopener"
            class="underline"
          >
            Binance API Management
          </a>
          с правом Read Info (без Trade и Withdraw).
        </p>
      </template>

      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div v-if="showSubmit" class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Отмена</Button>
        <Button type="submit" :disabled="createMutation.isPending.value">
          {{ createMutation.isPending.value ? "Создаём…" : "Создать" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
