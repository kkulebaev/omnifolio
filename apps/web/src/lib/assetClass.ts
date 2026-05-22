import { AssetClass } from "@/api/generated/model/assetClass";

export const ASSET_CLASS_LABELS = {
  ru_stock: "RU акции",
  us_stock: "US акции",
  ru_bond: "RU облигации",
  ru_etf: "RU ETF",
  us_etf: "US ETF",
  crypto: "Крипта",
  cash: "Наличные",
  real_estate: "Недвижимость",
  vehicle: "Транспорт",
  other_asset: "Прочее имущество",
} satisfies Record<AssetClass, string>;

export const ASSET_CLASS_SHORT_LABELS = {
  ru_stock: "ru акция",
  us_stock: "us акция",
  ru_bond: "ru обл.",
  ru_etf: "ru etf",
  us_etf: "us etf",
  crypto: "crypto",
  cash: "cash",
  real_estate: "недвиж.",
  vehicle: "транспорт",
  other_asset: "прочее",
} satisfies Record<AssetClass, string>;

export const PERSONAL_ASSET_CLASSES = ["real_estate", "vehicle", "other_asset"] as const;
