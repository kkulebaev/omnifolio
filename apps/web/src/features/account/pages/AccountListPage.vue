<script setup lang="ts">
import { ref } from "vue";
import { useListAccounts } from "@/api/generated";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/formatters";
import CreateAccountDialog from "../components/CreateAccountDialog.vue";

const accounts = useListAccounts();
const dialogOpen = ref(false);
</script>

<template>
  <!-- TODO(tw-arb): p-[22px] -->
  <div class="space-y-6 p-[22px]">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold">Аккаунты</h1>
      <Button @click="dialogOpen = true">Создать аккаунт</Button>
    </div>

    <p v-if="accounts.isLoading.value" class="text-sm opacity-60">Загрузка…</p>
    <p v-else-if="accounts.isError.value" class="text-sm text-red-600">Не удалось загрузить</p>

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
            <CardDescription>
              {{ a.type }} · создан {{ formatDate(a.createdAt) }}
            </CardDescription>
          </CardHeader>
        </Card>
      </RouterLink>
    </div>

    <CreateAccountDialog v-model:open="dialogOpen" />
  </div>
</template>
