import { ref } from "vue";

export interface ConfirmOptions {
  title: string;
  body?: string;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
}

interface ConfirmRequest {
  options: ConfirmOptions;
  resolver: (value: boolean) => void;
}

const current = ref<ConfirmRequest | null>(null);

export function confirm(options: ConfirmOptions): Promise<boolean> {
  current.value?.resolver(false);
  return new Promise<boolean>((resolve) => {
    current.value = { options, resolver: resolve };
  });
}

export function useConfirmState() {
  return {
    current,
    accept() {
      const r = current.value;
      current.value = null;
      r?.resolver(true);
    },
    cancel() {
      const r = current.value;
      current.value = null;
      r?.resolver(false);
    },
  };
}
