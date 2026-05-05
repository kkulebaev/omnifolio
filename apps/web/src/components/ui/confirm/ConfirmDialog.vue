<script setup lang="ts">
import { computed } from "vue";
import { useConfirmState } from "@/lib/confirm";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

const { current, accept, cancel } = useConfirmState();

const open = computed({
  get: () => current.value !== null,
  set: (v: boolean) => {
    if (!v) cancel();
  },
});
</script>

<template>
  <Dialog :open="open" @update:open="open = $event">
    <h2 class="text-lg font-semibold mb-2">{{ current?.options.title }}</h2>
    <p
      v-if="current?.options.body"
      class="text-sm text-muted-foreground mb-4 whitespace-pre-line"
    >
      {{ current.options.body }}
    </p>
    <div class="flex justify-end gap-2">
      <Button type="button" variant="outline" @click="cancel">
        {{ current?.options.cancelText ?? "Отмена" }}
      </Button>
      <Button
        type="button"
        :variant="current?.options.danger ? 'destructive' : 'default'"
        @click="accept"
      >
        {{ current?.options.confirmText ?? "Подтвердить" }}
      </Button>
    </div>
  </Dialog>
</template>
