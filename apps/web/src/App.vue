<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import AppLayout from "@/components/layout/AppLayout.vue";
import AuthLayout from "@/components/layout/AuthLayout.vue";

const auth = useAuthStore();
const route = useRoute();

onMounted(() => {
  auth.bootstrap();
});

const layout = computed(() => route.meta.layout ?? "app");
</script>

<template>
  <div v-if="!auth.ready" class="min-h-screen flex items-center justify-center text-sm opacity-60">
    Загрузка…
  </div>
  <AuthLayout v-else-if="layout === 'auth'">
    <RouterView />
  </AuthLayout>
  <AppLayout v-else>
    <RouterView />
  </AppLayout>
</template>
