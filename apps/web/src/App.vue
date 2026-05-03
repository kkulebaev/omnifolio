<script setup lang="ts">
import { ref } from "vue";
import { Button } from "@/components/ui/button";

const apiStatus = ref<string>("—");
const error = ref<string | null>(null);
const loading = ref(false);

async function checkHealth() {
  loading.value = true;
  error.value = null;
  try {
    const res = await fetch("/api/healthz");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = (await res.json()) as { status?: string };
    apiStatus.value = data.status ?? "unknown";
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <main class="min-h-screen flex items-center justify-center px-4">
    <div class="flex flex-col items-center gap-6 max-w-md w-full">
      <div class="text-center space-y-2">
        <h1 class="text-3xl font-bold tracking-tight">Omnifolio</h1>
        <p class="text-sm" style="color: hsl(var(--muted-foreground))">
          M0 — Skeleton
        </p>
      </div>

      <Button :disabled="loading" @click="checkHealth">
        {{ loading ? "Checking..." : "Check API" }}
      </Button>

      <div class="text-sm space-y-1 text-center">
        <p v-if="apiStatus !== '—'">
          API status:
          <code class="px-1.5 py-0.5 rounded" style="background-color: hsl(var(--muted))">
            {{ apiStatus }}
          </code>
        </p>
        <p v-if="error" class="text-red-600">Error: {{ error }}</p>
      </div>
    </div>
  </main>
</template>
