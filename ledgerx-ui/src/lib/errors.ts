import { AxiosError } from "axios";

// apiError turns an unknown thrown value into a human-readable string.
// The API returns plain-text bodies for errors (http.Error), so we surface
// those directly and fall back to status text or the JS error message.
export function apiError(err: unknown, fallback = "Something went wrong"): string {
  if (err instanceof AxiosError) {
    const data = err.response?.data;
    if (typeof data === "string" && data.trim()) return data.trim();
    if (data && typeof data === "object" && "detail" in data) return String((data as { detail: unknown }).detail);
    if (err.response?.statusText) return err.response.statusText;
    return err.message || fallback;
  }
  if (err instanceof Error) return err.message;
  return fallback;
}
