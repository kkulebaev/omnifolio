<script setup lang="ts">
import { ref } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import {
  useListAccounts,
  useCreateAccount,
  getListAccountsQueryKey,
} from "@/api/generated";
import { AccountType } from "@/api/generated/model/accountType";
import { Card, CardHeader, CardTitle, CardContent, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog } from "@/components/ui/dialog";
import { formatDate } from "@/lib/formatters";

const accounts = useListAccounts();
const createMutation = useCreateAccount();
const queryClient = useQueryClient();

const dialogOpen = ref(false);
const newName = ref("");
const formError = ref<string | null>(null);

async function submit() {
  formError.value = null;
  if (newName.value.trim().length === 0) {
    formError.value = "Название обязательно";
    return;
  }
  try {
    await createMutation.mutateAsync({
      data: { name: newName.value.trim(), type: AccountType.manual },
    });
    queryClient.invalidateQueries({ queryKey: getListAccountsQueryKey() });
    newName.value = "";
    dialogOpen.value = false;
  } catch (e) {
    formError.value = (e as Error).message;
  }
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold">Аккаунты</h1>
      <Button @click="dialogOpen = true">Создать аккаунт</Button>
    </div>

    <p v-if="accounts.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="accounts.isError.value" class="text-sm text-red-600">
      Не удалось загрузить
    </p>

    <Card v-else-if="!accounts.data.value?.items?.length">
      <CardContent class="py-12 text-center space-y-2">
        <p class="text-sm opacity-60">Пока нет аккаунтов</p>
        <Button @click="dialogOpen = true">Создать первый аккаунт</Button>
      </CardContent>
    </Card>

    <div v-else class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <RouterLink
        v-for="a in accounts.data.value!.items"
        :key="a.id"
        :to="{ name: 'account-detail', params: { id: a.id } }"
        class="block"
      >
        <Card class="hover:bg-muted/30 transition">
          <CardHeader>
            <CardTitle>{{ a.name }}</CardTitle>
            <CardDescription>{{ a.type }} · создан {{ formatDate(a.createdAt) }}</CardDescription>
          </CardHeader>
        </Card>
      </RouterLink>
    </div>

    <Dialog v-model:open="dialogOpen">
      <h2 class="text-lg font-semibold mb-4">Новый аккаунт</h2>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="acc-name">Название</Label>
          <Input id="acc-name" v-model="newName" placeholder="Например, Бумажные" />
        </div>
        <p class="text-sm opacity-60">Тип: <code>manual</code> (другие в M2+).</p>
        <p v-if="formError" class="text-sm text-red-600">{{ formError }}</p>
        <div class="flex justify-end gap-2">
          <Button type="button" variant="outline" @click="dialogOpen = false">Отмена</Button>
          <Button type="submit" :disabled="createMutation.isPending.value">
            {{ createMutation.isPending.value ? "Создаём…" : "Создать" }}
          </Button>
        </div>
      </form>
    </Dialog>
  </div>
</template>
