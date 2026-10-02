const API_URL = (import.meta.env.VITE_API_URL ?? "http://localhost:4000").replace(/\/$/, "");

/** An error response from the API: {"error": "..."} with an HTTP status. */
export class ApiError extends Error {
  readonly status: number;
  /** Seconds from the Retry-After header on 429 and 503, when present. */
  readonly retryAfter: number | null;

  constructor(status: number, message: string, retryAfter: number | null) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.retryAfter = retryAfter;
  }
}

type Params = Record<string, string | number | undefined>;

export async function apiGet<T>(path: string, params: Params = {}, signal?: AbortSignal): Promise<T> {
  const url = new URL(API_URL + path);
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) url.searchParams.set(key, String(value));
  }

  const response = await fetch(url, { signal, headers: { Accept: "application/json" } });
  if (!response.ok) {
    let message = `Request failed (${response.status})`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // Not JSON: keep the generic message.
    }
    const retry = Number(response.headers.get("Retry-After"));
    throw new ApiError(response.status, message, Number.isFinite(retry) && retry > 0 ? retry : null);
  }
  return (await response.json()) as T;
}
