import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { page, userEvent } from "vitest/browser";
import { mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { ref } from "vue";
import DepositFormDialog from "./DepositFormDialog.vue";
import type { Deposit } from "@/api/generated/model/deposit";

const mockCreate = vi.fn();
const mockUpdate = vi.fn();
const mockInvalidate = vi.fn();

vi.mock("@/api/generated", () => ({
  useCreateDeposit: () => ({ isPending: ref(false), mutateAsync: mockCreate }),
  useUpdateDeposit: () => ({ isPending: ref(false), mutateAsync: mockUpdate }),
  getListDepositsQueryKey: () => ["deposits"],
}));

vi.mock("@tanstack/vue-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/vue-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
  };
});

let wrapper: VueWrapper | undefined;

function mountDialog(props: Record<string, unknown> = {}) {
  // Dialog телепортируется в body: без attachTo форма вне документа и локаторам page не видна.
  wrapper = mount(DepositFormDialog, {
    props: { open: true, ...props },
    attachTo: document.body,
    global: { plugins: [createPinia(), VueQueryPlugin] },
  });
  return wrapper;
}

const EXISTING: Deposit = {
  id: "0190a000-0000-7000-8000-000000000001",
  date: "2025-03-14",
  kind: "withdrawal",
  currency: "USD",
  amount: "123.45",
  createdAt: "2025-03-14T10:00:00Z",
  updatedAt: "2025-03-14T10:00:00Z",
};

const AMOUNT_ERROR = "Положительное число, не больше 2 знаков после запятой";

const amountInput = () => page.getByLabelText("Сумма");
const submitButton = (name: "Создать" | "Сохранить") => page.getByRole("button", { name });

describe("DepositFormDialog", () => {
  beforeEach(() => {
    mockCreate.mockReset();
    mockUpdate.mockReset();
    mockInvalidate.mockReset();
  });

  afterEach(() => {
    wrapper?.unmount();
    wrapper = undefined;
  });

  it.each(["", "abc", "0", "1.234", "-5"])(
    "rejects amount %j without calling the API",
    async (value) => {
      mountDialog();
      await userEvent.fill(amountInput(), value);
      await userEvent.click(submitButton("Создать"));

      await expect.element(page.getByText(AMOUNT_ERROR)).toBeVisible();
      expect(mockCreate).not.toHaveBeenCalled();
    },
  );

  it("accepts a comma as the decimal separator", async () => {
    mockCreate.mockResolvedValueOnce({});
    const wrapper = mountDialog();
    await userEvent.fill(page.getByLabelText("Дата"), "2025-04-01");
    await userEvent.fill(amountInput(), "1 500,5");
    await userEvent.click(submitButton("Создать"));

    await expect.poll(() => wrapper.emitted("update:open")?.at(-1)).toEqual([false]);
    expect(mockCreate).toHaveBeenCalledWith({
      data: { date: "2025-04-01", kind: "deposit", currency: "RUB", amount: "1500.5" },
    });
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["portfolio", "returns"] });
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["portfolio", "history"] });
  });

  it("prefills fields in edit mode and submits an update", async () => {
    mockUpdate.mockResolvedValueOnce({});
    mountDialog({ deposit: EXISTING });

    await expect.element(page.getByLabelText("Дата")).toHaveValue("2025-03-14");
    await expect.element(amountInput()).toHaveValue("123.45");
    await expect.element(page.getByLabelText("Валюта")).toHaveValue("USD");
    await expect.element(page.getByRole("radio", { name: "Вывод" })).toBeChecked();

    await userEvent.fill(amountInput(), "200");
    await userEvent.click(submitButton("Сохранить"));

    await expect.poll(() => mockUpdate.mock.calls.length).toBe(1);
    expect(mockCreate).not.toHaveBeenCalled();
    expect(mockUpdate).toHaveBeenCalledWith({
      depositId: EXISTING.id,
      data: { date: "2025-03-14", kind: "withdrawal", currency: "USD", amount: "200" },
    });
  });

  it("switches kind to withdrawal for a new record", async () => {
    mockCreate.mockResolvedValueOnce({});
    mountDialog();
    await userEvent.click(page.getByRole("radio", { name: "Вывод" }));
    await userEvent.fill(amountInput(), "10");
    await userEvent.click(submitButton("Создать"));

    await expect.poll(() => mockCreate.mock.calls.length).toBe(1);
    expect(mockCreate.mock.calls[0]?.[0].data.kind).toBe("withdrawal");
  });
});
