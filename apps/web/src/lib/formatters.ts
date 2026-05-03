export function formatCurrency(amount: string | number, currency: string): string {
  const num = typeof amount === "string" ? Number(amount) : amount;
  if (!Number.isFinite(num)) return "—";
  try {
    return new Intl.NumberFormat("ru-RU", {
      style: "currency",
      currency: ["USDT", "USDC", "BUSD"].includes(currency) ? "USD" : currency,
      maximumFractionDigits: 2,
    }).format(num);
  } catch {
    return `${num.toFixed(2)} ${currency}`;
  }
}

export function formatCompact(amount: string | number, currency: string): string {
  const num = typeof amount === "string" ? Number(amount) : amount;
  if (!Number.isFinite(num)) return "—";
  const sym = currencySymbol(currency);
  if (Math.abs(num) >= 1_000_000) {
    return (
      new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 }).format(num / 1_000_000) +
      ` млн ${sym}`
    );
  }
  if (Math.abs(num) >= 1_000) {
    return (
      new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 }).format(num / 1_000) +
      ` тыс ${sym}`
    );
  }
  return formatCurrency(num, currency);
}

function currencySymbol(c: string): string {
  switch (c) {
    case "RUB":
      return "₽";
    case "USD":
    case "USDT":
    case "USDC":
    case "BUSD":
      return "$";
    case "EUR":
      return "€";
    default:
      return c;
  }
}

export function formatQuantity(amount: string | number): string {
  const num = typeof amount === "string" ? Number(amount) : amount;
  if (!Number.isFinite(num)) return "—";
  return new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 8 }).format(num);
}

export function formatNumber(amount: string | number, fractionDigits = 2): string {
  const num = typeof amount === "string" ? Number(amount) : amount;
  if (!Number.isFinite(num)) return "—";
  return new Intl.NumberFormat("ru-RU", {
    maximumFractionDigits: fractionDigits,
    minimumFractionDigits: 0,
  }).format(num);
}

export function formatPercent(part: number, total: number): string {
  if (total === 0) return "—";
  return new Intl.NumberFormat("ru-RU", {
    style: "percent",
    maximumFractionDigits: 1,
  }).format(part / total);
}

export function formatDate(d: Date | string): string {
  const date = typeof d === "string" ? new Date(d) : d;
  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

const rtf = new Intl.RelativeTimeFormat("ru-RU", { numeric: "auto" });

export function formatRelative(d: Date | string | null | undefined): string {
  if (!d) return "—";
  const date = typeof d === "string" ? new Date(d) : d;
  const diffSec = Math.round((date.getTime() - Date.now()) / 1000);
  const abs = Math.abs(diffSec);
  if (abs < 60) return rtf.format(diffSec, "second");
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), "hour");
  return rtf.format(Math.round(diffSec / 86400), "day");
}

// priceAgeColor returns Tailwind text color class based on how old a price is.
// fresh (<5min) green, ok (<30min) default, stale (<2h) orange, expired red.
export function priceAgeColor(d: Date | string | null | undefined, isStale = false): string {
  if (!d || isStale) return "text-red-600";
  const ageSec = Math.abs(Date.now() - new Date(d).getTime()) / 1000;
  if (ageSec < 300) return "text-green-600";
  if (ageSec < 1800) return "";
  if (ageSec < 7200) return "text-orange-500";
  return "text-red-600";
}
