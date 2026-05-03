export type Problem = {
  type?: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
  fields?: Record<string, string>;
};

export class HttpError extends Error {
  status: number;
  problem: Problem;
  constructor(status: number, problem: Problem) {
    super(problem.title || `HTTP ${status}`);
    this.name = "HttpError";
    this.status = status;
    this.problem = problem;
  }
}

export type FetcherArgs = {
  url: string;
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  params?: Record<string, unknown>;
  headers?: Record<string, string>;
  data?: unknown;
  signal?: AbortSignal;
};

const API_BASE = "/api";

export const fetcher = async <T>(args: FetcherArgs): Promise<T> => {
  const qs = args.params
    ? "?" +
      new URLSearchParams(
        Object.entries(args.params)
          .filter(([, v]) => v !== undefined && v !== null)
          .map(([k, v]) => [k, String(v)]),
      ).toString()
    : "";

  const res = await fetch(`${API_BASE}${args.url}${qs}`, {
    method: args.method,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(args.headers ?? {}),
    },
    body: args.data !== undefined ? JSON.stringify(args.data) : undefined,
    signal: args.signal,
  });

  if (!res.ok) {
    let problem: Problem;
    try {
      problem = (await res.json()) as Problem;
    } catch {
      problem = { title: res.statusText, status: res.status };
    }
    throw new HttpError(res.status, problem);
  }

  if (res.status === 204) return undefined as T;
  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("application/json")) return (await res.json()) as T;
  return undefined as T;
};
