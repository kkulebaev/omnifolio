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

const API_BASE = "/api";

export const fetcher = async <T>(
  url: string,
  options: RequestInit,
): Promise<T> => {
  const res = await fetch(`${API_BASE}${url}`, {
    credentials: "include",
    ...options,
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
