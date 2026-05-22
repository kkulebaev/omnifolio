import { describe, it, expect } from "vitest";
import { AssetClass } from "@/api/generated/model/assetClass";
import { ASSET_CLASS_LABELS, ASSET_CLASS_SHORT_LABELS } from "@/lib/assetClass";

const ALL_VALUES = Object.values(AssetClass) as AssetClass[];

describe("ASSET_CLASS_LABELS exhaustiveness", () => {
  it("has a non-empty label for every AssetClass value", () => {
    for (const cls of ALL_VALUES) {
      const label = ASSET_CLASS_LABELS[cls];
      expect(label, `ASSET_CLASS_LABELS["${cls}"] should be defined`).toBeTruthy();
      expect(typeof label).toBe("string");
      expect(label.trim().length).toBeGreaterThan(0);
    }
  });
});

describe("ASSET_CLASS_SHORT_LABELS exhaustiveness", () => {
  it("has a non-empty short label for every AssetClass value", () => {
    for (const cls of ALL_VALUES) {
      const label = ASSET_CLASS_SHORT_LABELS[cls];
      expect(label, `ASSET_CLASS_SHORT_LABELS["${cls}"] should be defined`).toBeTruthy();
      expect(typeof label).toBe("string");
      expect(label.trim().length).toBeGreaterThan(0);
    }
  });
});
