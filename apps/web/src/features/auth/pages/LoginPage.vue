<script setup lang="ts">
import { ref } from "vue";
import { useRouter, useRoute } from "vue-router";
import { useQueryClient } from "@tanstack/vue-query";
import { useLogin } from "@/api/generated";
import { useAuthStore } from "@/stores/auth";
import { HttpError } from "@/api/mutator";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";

const email = ref("");
const password = ref("");
const rememberMe = ref(false);
const error = ref<string | null>(null);

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();
const queryClient = useQueryClient();
const login = useLogin();

async function submit() {
  error.value = null;
  try {
    const user = await login.mutateAsync({
      data: { email: email.value, password: password.value, rememberMe: rememberMe.value },
    });
    auth.setUser(user);
    queryClient.setQueryData(["/auth/me"], user);
    const redirect = (route.query.redirect as string) ?? "/";
    router.push(redirect);
  } catch (e) {
    if (e instanceof HttpError) {
      if (e.status === 401) error.value = "Неверный email или пароль";
      else if (e.status === 422) error.value = "Проверьте корректность полей";
      else error.value = e.problem.title || "Ошибка";
    } else {
      error.value = "Ошибка сети";
    }
  }
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>Вход в Omnifolio</CardTitle>
    </CardHeader>
    <CardContent>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <Label for="email">Email</Label>
          <Input
            id="email"
            v-model="email"
            type="email"
            autocomplete="email"
            placeholder="dev@local.test"
            :disabled="login.isPending.value"
          />
        </div>
        <div class="space-y-1.5">
          <Label for="password">Пароль</Label>
          <Input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            :disabled="login.isPending.value"
          />
        </div>
        <div class="flex items-center gap-2">
          <input
            id="rememberMe"
            v-model="rememberMe"
            type="checkbox"
            class="h-4 w-4 rounded border-input text-primary focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="login.isPending.value"
          />
          <Label for="rememberMe" class="cursor-pointer select-none">Запомнить меня</Label>
        </div>
        <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
        <Button type="submit" class="w-full" :disabled="login.isPending.value">
          {{ login.isPending.value ? "Вход…" : "Войти" }}
        </Button>
      </form>
    </CardContent>
  </Card>
</template>
