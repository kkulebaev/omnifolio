import type { QueryClient } from "@tanstack/vue-query";
import { getListDepositsQueryKey } from "@/api/generated";

// Returns and the "invested" line on the history chart are derived from the
// deposits journal, so every journal change must refresh them too. The
// `portfolio` value itself does not depend on deposits, hence the narrower keys.
export function invalidateDepositQueries(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: getListDepositsQueryKey() });
  queryClient.invalidateQueries({ queryKey: ["portfolio", "returns"] });
  queryClient.invalidateQueries({ queryKey: ["portfolio", "history"] });
}
