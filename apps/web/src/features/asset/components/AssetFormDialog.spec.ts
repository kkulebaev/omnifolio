import { describe, it, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createPinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { ref } from "vue";
import AssetFormDialog from "./AssetFormDialog.vue";
import { HttpError } from "@/api/mutator";

// ---------------------------------------------------------------------------
// Module mocks — hoisted by vitest before imports
// ---------------------------------------------------------------------------

const mockCreate = vi.fn();
const mockInvalidate = vi.fn();

vi.mock("@/api/generated", () => ({
  useCreateInstrument: () => ({
    isPending: ref(false),
    mutateAsync: mockCreate,
  }),
  useUpdateInstrument: () => ({
    isPending: ref(false),
    mutateAsync: vi.fn(),
  }),
  getListInstrumentsQueryKey: () => ["instruments"],
}));

vi.mock("@tanstack/vue-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/vue-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
  };
});

// ---------------------------------------------------------------------------
// Stubs for UI primitives so we get real HTML elements in the DOM tree
// ---------------------------------------------------------------------------

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

function mountDialog(extraProps: Record<string, unknown> = {}) {
  return mount(AssetFormDialog, {
    props: { open: true, ...extraProps },
    global: {
      plugins: [createPinia(), VueQueryPlugin],
      stubs: GLOBAL_STUBS,
    },
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("AssetFormDialog — validation", () => {
  beforeEach(() => {
    mockCreate.mockReset();
    mockInvalidate.mockReset();
  });

  it("shows validation errors when submitting an empty form", async () => {
    const wrapper = mountDialog();

    await wrapper.find("form").trigger("submit");
    await flushPromises();

    const html = wrapper.html();
    // name is empty → "Обязательно, до 100 символов"
    expect(html).toContain("Обязательно");
    // ticker is empty → fails /^[A-Za-z0-9_-]{1,32}$/ regex
    expect(html).toContain("Латиница");
    // initialPrice is empty → "Укажи положительное число"
    expect(html).toContain("Укажи положительное");
  });

  it("shows ticker validation error when ticker contains invalid characters", async () => {
    const wrapper = mountDialog();
    const inputs = wrapper.findAll("input");

    // Provide a valid name so only the ticker path is exercised
    await inputs[0].setValue("My Apartment");
    // Ticker with a space is invalid per /^[A-Za-z0-9_-]{1,32}$/
    await inputs[1].setValue("AB CD");

    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(wrapper.html()).toContain("Латиница");
  });

  it("shows conflict error on the ticker field when the API returns 409", async () => {
    const wrapper = mountDialog();
    const inputs = wrapper.findAll("input");

    // inputs[0] = name, inputs[1] = ticker, inputs[2] = currency, inputs[3] = price
    // currency (inputs[2]) is pre-filled with "RUB" from the ui store default
    await inputs[0].setValue("My Apartment");
    await inputs[1].setValue("MYAPT123");
    await inputs[3].setValue("5000000");

    mockCreate.mockRejectedValueOnce(
      new HttpError(409, { title: "Conflict", status: 409 }),
    );

    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(wrapper.html()).toContain("Уже есть актив с таким тикером");
  });
});
