import { ref, watch } from "vue";
import { defineStore } from "pinia";
import { useToggle } from "@vueuse/core";

const SUPPORTED_CURRENCIES = ["RUB", "USD", "EUR"] as const;
export type DisplayCurrency = (typeof SUPPORTED_CURRENCIES)[number];

const LS_CURRENCY = "omnifolio:displayCurrency";
const LS_THEME = "omnifolio:theme";
const LS_PRIVACY = "omnifolio:privacy";
const LS_MERGE_POSITIONS = "omnifolio:mergePositions";
const LS_DEFAULT_DEPOSIT = "omnifolio:defaultDepositAmount";

export const useUiStore = defineStore("ui", () => {
  const stored = (localStorage.getItem(LS_CURRENCY) as DisplayCurrency | null) ?? "RUB";
  const displayCurrency = ref<DisplayCurrency>(
    SUPPORTED_CURRENCIES.includes(stored) ? stored : "RUB",
  );

  watch(displayCurrency, (v) => {
    localStorage.setItem(LS_CURRENCY, v);
  });

  const storedTheme =
    (localStorage.getItem(LS_THEME) as "light" | "dark" | null) ?? "light";
  const [theme, toggleTheme] = useToggle<"dark", "light">(storedTheme, {
    truthyValue: "dark",
    falsyValue: "light",
  });

  function applyTheme() {
    if (theme.value === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  }
  applyTheme();
  watch(theme, () => {
    localStorage.setItem(LS_THEME, theme.value);
    applyTheme();
  });

  const [privacy, togglePrivacy] = useToggle(
    localStorage.getItem(LS_PRIVACY) === "1",
  );
  watch(privacy, (v) => {
    localStorage.setItem(LS_PRIVACY, v ? "1" : "0");
  });

  const [mergePositions, toggleMergePositions] = useToggle(
    localStorage.getItem(LS_MERGE_POSITIONS) === "1",
  );
  watch(mergePositions, (v) => {
    localStorage.setItem(LS_MERGE_POSITIONS, v ? "1" : "0");
  });

  const storedDeposit = localStorage.getItem(LS_DEFAULT_DEPOSIT);
  const defaultDepositAmount = ref<number | null>(
    storedDeposit && /^[1-9][0-9]*$/.test(storedDeposit)
      ? Number(storedDeposit)
      : null,
  );
  watch(defaultDepositAmount, (v) => {
    if (v == null) {
      localStorage.removeItem(LS_DEFAULT_DEPOSIT);
    } else {
      localStorage.setItem(LS_DEFAULT_DEPOSIT, String(v));
    }
  });

  const [mobileSidebarOpen, toggleMobileSidebar] = useToggle(false);

  return {
    displayCurrency,
    theme,
    toggleTheme,
    privacy,
    togglePrivacy,
    mergePositions,
    toggleMergePositions,
    defaultDepositAmount,
    mobileSidebarOpen,
    toggleMobileSidebar,
    SUPPORTED_CURRENCIES,
  };
});
