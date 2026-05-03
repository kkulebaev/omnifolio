import { ref, watch } from "vue";
import { defineStore } from "pinia";

const SUPPORTED_CURRENCIES = ["RUB", "USD", "EUR"] as const;
export type DisplayCurrency = (typeof SUPPORTED_CURRENCIES)[number];

const LS_CURRENCY = "omnifolio:displayCurrency";
const LS_THEME = "omnifolio:theme";

export const useUiStore = defineStore("ui", () => {
  const stored = (localStorage.getItem(LS_CURRENCY) as DisplayCurrency | null) ?? "RUB";
  const displayCurrency = ref<DisplayCurrency>(
    SUPPORTED_CURRENCIES.includes(stored) ? stored : "RUB",
  );

  watch(displayCurrency, (v) => {
    localStorage.setItem(LS_CURRENCY, v);
  });

  const storedTheme = (localStorage.getItem(LS_THEME) as "light" | "dark" | null) ?? "light";
  const theme = ref<"light" | "dark">(storedTheme);

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

  function toggleTheme() {
    theme.value = theme.value === "light" ? "dark" : "light";
  }

  return {
    displayCurrency,
    theme,
    toggleTheme,
    SUPPORTED_CURRENCIES,
  };
});
