import { describe, it, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
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

const inputStub = {
  inheritAttrs: false,
  template: `<input v-bind="$attrs" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
  props: ["modelValue"],
  emits: ["update:modelValue", "input"],
};

const GLOBAL_STUBS = {
  Dialog: {
    template: '<div data-stub="dialog"><slot /></div>',
    props: ["open"],
    emits: ["update:open"],
  },
  Button: {
    template: '<button :type="type ?? \'button\'" :disabled="disabled ?? false"><slot /></button>',
    props: ["type", "disabled", "variant"],
  },
  Input: inputStub,
  Label: { template: "<label><slot /></label>" },
};

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(DepositFormDialog, {
    props: { open: true, ...props },
    global: {
      plugins: [createPinia(), VueQueryPlugin],
      stubs: GLOBAL_STUBS,
    },
  });
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

describe("DepositFormDialog", () => {
  beforeEach(() => {
    mockCreate.mockReset();
    mockUpdate.mockReset();
    mockInvalidate.mockReset();
  });

  it.each(["", "abc", "0", "1.234", "-5"])(
    "rejects amount %j without calling the API",
    async (value) => {
      const wrapper = mountDialog();
      await wrapper.find("#dep-amount").setValue(value);
      await wrapper.find("form").trigger("submit");
      await flushPromises();

      expect(wrapper.html()).toContain("не больше 2 знаков");
      expect(mockCreate).not.toHaveBeenCalled();
    },
  );

  it("accepts a comma as the decimal separator", async () => {
    mockCreate.mockResolvedValueOnce({});
    const wrapper = mountDialog();
    await wrapper.find("#dep-date").setValue("2025-04-01");
    await wrapper.find("#dep-amount").setValue("1 500,5");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(mockCreate).toHaveBeenCalledWith({
      data: { date: "2025-04-01", kind: "deposit", currency: "RUB", amount: "1500.5" },
    });
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["portfolio", "returns"] });
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["portfolio", "history"] });
    expect(wrapper.emitted("update:open")?.at(-1)).toEqual([false]);
  });

  it("prefills fields in edit mode and submits an update", async () => {
    mockUpdate.mockResolvedValueOnce({});
    const wrapper = mountDialog({ deposit: EXISTING });

    expect((wrapper.find("#dep-date").element as HTMLInputElement).value).toBe("2025-03-14");
    expect((wrapper.find("#dep-amount").element as HTMLInputElement).value).toBe("123.45");
    expect((wrapper.find("#dep-currency").element as HTMLSelectElement).value).toBe("USD");
    expect(wrapper.find('[data-kind="withdrawal"]').attributes("aria-checked")).toBe("true");

    await wrapper.find("#dep-amount").setValue("200");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(mockCreate).not.toHaveBeenCalled();
    expect(mockUpdate).toHaveBeenCalledWith({
      depositId: EXISTING.id,
      data: { date: "2025-03-14", kind: "withdrawal", currency: "USD", amount: "200" },
    });
  });

  it("switches kind to withdrawal for a new record", async () => {
    mockCreate.mockResolvedValueOnce({});
    const wrapper = mountDialog();
    await wrapper.find('[data-kind="withdrawal"]').trigger("click");
    await wrapper.find("#dep-amount").setValue("10");
    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(mockCreate.mock.calls[0]?.[0].data.kind).toBe("withdrawal");
  });
});
