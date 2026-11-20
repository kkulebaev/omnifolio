import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { page, userEvent } from "vitest/browser";
import { mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { ref } from "vue";
import UpdateCredentialsDialog from "./UpdateCredentialsDialog.vue";
import { HttpError } from "@/api/mutator";
import type { AccountType } from "@/api/generated/model/accountType";

const mockUpdate = vi.fn();
const mockInvalidate = vi.fn();

vi.mock("@/api/generated", () => ({
  useUpdateAccountCredentials: () => ({ isPending: ref(false), mutateAsync: mockUpdate }),
  getGetAccountQueryKey: (id: string) => ["account", id],
  getListAccountsQueryKey: () => ["accounts"],
}));

vi.mock("@tanstack/vue-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/vue-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
  };
});

const ACCOUNT_ID = "0190a000-0000-7000-8000-000000000001";

let wrapper: VueWrapper | undefined;

function mountDialog(accountType: AccountType) {
  wrapper = mount(UpdateCredentialsDialog, {
    props: { open: true, accountId: ACCOUNT_ID, accountType },
    attachTo: document.body,
    global: { plugins: [createPinia(), VueQueryPlugin] },
  });
  return wrapper;
}

describe("UpdateCredentialsDialog", () => {
  beforeEach(() => {
    mockUpdate.mockReset();
    mockInvalidate.mockReset();
  });

  afterEach(() => {
    wrapper?.unmount();
    wrapper = undefined;
  });

  it("sends api key and secret for bybit and closes", async () => {
    mockUpdate.mockResolvedValueOnce({});
    const w = mountDialog("bybit");
    await userEvent.fill(page.getByLabelText("API Key"), "newkey123");
    await userEvent.fill(page.getByLabelText("API Secret"), "newsecret123");
    await userEvent.click(page.getByRole("button", { name: "Сохранить" }));

    await expect.poll(() => w.emitted("update:open")?.at(-1)).toEqual([false]);
    expect(mockUpdate).toHaveBeenCalledWith({
      accountId: ACCOUNT_ID,
      data: { apiKey: "newkey123", apiSecret: "newsecret123" },
    });
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["account", ACCOUNT_ID] });
  });

  it("sends only the token for tinvest", async () => {
    mockUpdate.mockResolvedValueOnce({});
    mountDialog("tinvest");
    await userEvent.fill(page.getByLabelText("T-Invest токен"), "t.0123456789");
    await userEvent.click(page.getByRole("button", { name: "Сохранить" }));

    await expect.poll(() => mockUpdate.mock.calls.length).toBe(1);
    expect(mockUpdate.mock.calls[0]?.[0].data).toEqual({ token: "t.0123456789" });
  });

  it("does not call the API with a short secret", async () => {
    mountDialog("bybit");
    await userEvent.fill(page.getByLabelText("API Key"), "newkey123");
    await userEvent.fill(page.getByLabelText("API Secret"), "short");
    await userEvent.click(page.getByRole("button", { name: "Сохранить" }));

    await expect.element(page.getByText("API key и secret обязательны")).toBeVisible();
    expect(mockUpdate).not.toHaveBeenCalled();
  });

  it("shows a rejection and stays open when the source refuses the key", async () => {
    mockUpdate.mockRejectedValueOnce(
      new HttpError(422, { title: "Invalid credentials", status: 422, fields: { apiKey: "rejected by Bybit" } }),
    );
    const w = mountDialog("bybit");
    await userEvent.fill(page.getByLabelText("API Key"), "newkey123");
    await userEvent.fill(page.getByLabelText("API Secret"), "newsecret123");
    await userEvent.click(page.getByRole("button", { name: "Сохранить" }));

    await expect.element(page.getByText("Ключ отклонён Bybit. Проверь правильность и права.")).toBeVisible();
    expect(w.emitted("update:open")).toBeUndefined();
  });
});
