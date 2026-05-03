import { computed, ref } from "vue";
import { defineStore } from "pinia";
import { getMe } from "@/api/generated";
import type { User } from "@/api/generated/model/user";
import { HttpError } from "@/api/mutator";

export const useAuthStore = defineStore("auth", () => {
  const user = ref<User | null>(null);
  const ready = ref(false);
  let resolveReady: () => void;
  const readyPromise = new Promise<void>((r) => {
    resolveReady = r;
  });

  const isAuthenticated = computed(() => user.value !== null);

  async function bootstrap() {
    try {
      const me = await getMe();
      user.value = me;
    } catch (err) {
      if (err instanceof HttpError && err.status === 401) {
        user.value = null;
      } else {
        console.error("auth bootstrap failed", err);
        user.value = null;
      }
    } finally {
      ready.value = true;
      resolveReady();
    }
  }

  function setUser(u: User | null) {
    user.value = u;
  }

  function clear() {
    user.value = null;
  }

  return {
    user,
    isAuthenticated,
    ready,
    readyPromise,
    bootstrap,
    setUser,
    clear,
  };
});
