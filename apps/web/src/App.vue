<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import AppLayout from "@/components/layout/AppLayout.vue";
import AuthLayout from "@/components/layout/AuthLayout.vue";

const route = useRoute();

const layout = computed(() => route.meta.layout ?? "app");
</script>

<template>
  <AuthLayout v-if="layout === 'auth'">
    <RouterView />
  </AuthLayout>
  <AppLayout v-else>
    <RouterView />
  </AppLayout>
</template>
