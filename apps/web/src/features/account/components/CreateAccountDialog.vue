<script setup lang="ts">
import { ref, watch } from "vue";
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

const tab = ref<"manual" | "tinvest">("manual");

// Manual tab state
const manualName = ref("");

// T-Invest tab state
const tToken = ref("");
const tSubAccounts = ref<TInvestSubAccount[]>([]);
const tSelectedId = ref<string>("");
const tName = ref("");
const tStep = ref<"token" | "select">("token");
const previewLoading = ref(false);

const error = ref<string | null>(null);

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
    } else {
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
    }
    queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
    emit("update:open", false);
  } catch (e) {
    error.value = "Не удалось создать: " + (e as Error).message;
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
    </div>

    <form class="space-y-4" @submit.prevent="submit">
      <template v-if="tab === 'manual'">
        <div class="space-y-1.5">
          <Label for="acc-name">Название</Label>
          <Input id="acc-name" v-model="manualName" placeholder="Например, Бумажные" />
        </div>
        <p class="text-sm opacity-60">Тип: <code>manual</code> — позиции добавляются вручную.</p>
      </template>

      <template v-else>
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

      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div v-if="tab === 'manual' || tStep === 'select'" class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="emit('update:open', false)">Отмена</Button>
        <Button type="submit" :disabled="createMutation.isPending.value">
          {{ createMutation.isPending.value ? "Создаём…" : "Создать" }}
        </Button>
      </div>
    </form>
  </Dialog>
</template>
