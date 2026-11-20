import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { page, userEvent } from "vitest/browser";
import { mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { ref } from "vue";
import AssetFormDialog from "./AssetFormDialog.vue";
import { HttpError } from "@/api/mutator";

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

let wrapper: VueWrapper | undefined;

function mountDialog(extraProps: Record<string, unknown> = {}) {
  // Dialog телепортируется в body: без attachTo форма вне документа и локаторам page не видна.
  wrapper = mount(AssetFormDialog, {
    props: { open: true, ...extraProps },
    attachTo: document.body,
    global: { plugins: [createPinia(), VueQueryPlugin] },
  });
  return wrapper;
}

const submit = () => userEvent.click(page.getByRole("button", { name: "Создать" }));

describe("AssetFormDialog — validation", () => {
  beforeEach(() => {
    mockCreate.mockReset();
    mockInvalidate.mockReset();
  });

  afterEach(() => {
    wrapper?.unmount();
    wrapper = undefined;
  });

  it("shows validation errors when submitting an empty form", async () => {
    mountDialog();

    await submit();

    await expect.element(page.getByText("Обязательно, до 100 символов")).toBeVisible();
    await expect
      .element(page.getByText("Латиница, цифры, дефис/подчёркивание, до 32 знаков"))
      .toBeVisible();
    await expect.element(page.getByText("Укажи положительное число")).toBeVisible();
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("shows ticker validation error when ticker contains invalid characters", async () => {
    mountDialog();

    await userEvent.fill(page.getByLabelText("Название"), "My Apartment");
    await userEvent.fill(page.getByLabelText("Тикер"), "AB CD");
    await submit();

    await expect
      .element(page.getByText("Латиница, цифры, дефис/подчёркивание, до 32 знаков"))
      .toBeVisible();
    await expect.element(page.getByText("Обязательно, до 100 символов")).not.toBeInTheDocument();
  });

  it("shows conflict error on the ticker field when the API returns 409", async () => {
    mockCreate.mockRejectedValueOnce(new HttpError(409, { title: "Conflict", status: 409 }));
    mountDialog();

    await userEvent.fill(page.getByLabelText("Название"), "My Apartment");
    await userEvent.fill(page.getByLabelText("Тикер"), "MYAPT123");
    await userEvent.fill(page.getByLabelText("Начальная цена"), "5000000");
    await submit();

    await expect.element(page.getByText("Уже есть актив с таким тикером")).toBeVisible();
    expect(mockCreate).toHaveBeenCalledOnce();
  });
});
