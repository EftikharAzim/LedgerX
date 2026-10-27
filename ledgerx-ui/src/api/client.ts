import axios, { CanceledError, type AxiosError, type InternalAxiosRequestConfig } from "axios";
import { endSession } from "../app/auth-session.ts";
import {
  getAccess, getRefresh, getSession, isCurrentSession, rotateTokens,
  type AuthSession, type Tokens,
} from "./tokens.ts";

type SessionRequest = InternalAxiosRequestConfig & {
  _retried?: boolean;
  _session?: AuthSession;
};

function requireCurrentSession(config: SessionRequest) {
  if (config._session && !isCurrentSession(config._session)) {
    const error = new CanceledError("Authentication session changed");
    error.config = config;
    throw error;
  }
}

const api = axios.create({
  baseURL: import.meta.env?.VITE_API_BASE_URL,
  timeout: 10000,
});

api.interceptors.request.use((config: SessionRequest) => {
  if (!config.url?.includes("/auth/")) {
    config._session ??= getSession();
    requireCurrentSession(config);
    // All private requests end when the user logs out or changes accounts,
    // including requests whose query functions do not consume a query signal.
    config.signal = config.signal instanceof AbortSignal
      ? AbortSignal.any([config.signal, config._session.signal])
      : config._session.signal;
  }
  const token = getAccess();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  else config.headers.delete("Authorization");
  return config;
});

// Single-flight refresh: many requests can 401 at once when the access token
// expires; they all await the same refresh call rather than stampeding it.
let refreshing: { session: AuthSession; promise: Promise<string | null> } | null = null;

async function refreshAccessToken(session: AuthSession): Promise<string | null> {
  const refresh_token = getRefresh();
  if (!refresh_token) return null;
  try {
    // Bare axios (not `api`) so this request skips the interceptors below.
    const res = await axios.post(`${api.defaults.baseURL ?? ""}/auth/refresh`, { refresh_token }, {
      timeout: api.defaults.timeout,
    });
    const t = res.data as Tokens;
    if (!rotateTokens(t, session)) {
      // Let an in-flight refresh finish so its successor can be revoked even
      // when logout or another login happened while the request was running.
      await axios.post(`${api.defaults.baseURL ?? ""}/auth/logout`, {
        refresh_token: t.refresh_token,
      }, { timeout: api.defaults.timeout }).catch(() => {});
      return null;
    }
    return t.access_token;
  } catch {
    if (isCurrentSession(session)) endSession();
    return null;
  }
}

api.interceptors.response.use(
  (response) => {
    requireCurrentSession(response.config as SessionRequest);
    return response;
  },
  async (error: AxiosError) => {
    const original = error.config as SessionRequest | undefined;
    if (original) requireCurrentSession(original);
    const url = original?.url ?? "";
    // Only attempt a refresh once per request, and never for the auth calls
    // themselves (a failed refresh/login shouldn't recurse).
    const isAuthCall = url.includes("/auth/");
    if (error.response?.status === 401 && original && !original._retried && !isAuthCall && getRefresh()) {
      original._retried = true;
      const session = original._session ?? getSession();
      if (!refreshing || refreshing.session !== session) {
        refreshing = { session, promise: refreshAccessToken(session) };
      }
      const attempt = refreshing;
      let newAccess: string | null;
      try {
        newAccess = await attempt.promise;
      } finally {
        if (refreshing === attempt) refreshing = null;
      }
      requireCurrentSession(original);
      if (newAccess) {
        original.headers.Authorization = `Bearer ${newAccess}`;
        return api(original);
      }
      // Refresh failure ends the session; AuthProvider updates the route guard.
    }
    return Promise.reject(error);
  },
);

export default api;
