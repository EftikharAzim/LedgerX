// Single source of truth for auth tokens in localStorage. The access token is
// short-lived and sent on every request; the refresh token is long-lived and
// used only to mint new access tokens.
const ACCESS = "access_token";
const REFRESH = "refresh_token";

export type Tokens = { access_token: string; refresh_token: string };

export type AuthSession = { readonly id: number; readonly signal: AbortSignal };

let sessionController = new AbortController();
let session: AuthSession = { id: 0, signal: sessionController.signal };
const listeners = new Set<() => void>();

export function getSession() {
  return session;
}

export function isCurrentSession(candidate: AuthSession) {
  return candidate.id === session.id;
}

export function subscribeAuth(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

function replaceSession() {
  sessionController.abort();
  sessionController = new AbortController();
  session = { id: session.id + 1, signal: sessionController.signal };
  listeners.forEach((listener) => listener());
}

export function getAccess() {
  return localStorage.getItem(ACCESS);
}
export function getRefresh() {
  return localStorage.getItem(REFRESH);
}
export function setTokens(t: Tokens) {
  localStorage.setItem(ACCESS, t.access_token);
  localStorage.setItem(REFRESH, t.refresh_token);
  replaceSession();
}

// Rotation keeps requests in the same session alive. A response from an
// earlier login must never replace the current user's credentials.
export function rotateTokens(t: Tokens, expectedSession: AuthSession) {
  if (!isCurrentSession(expectedSession)) return false;
  localStorage.setItem(ACCESS, t.access_token);
  localStorage.setItem(REFRESH, t.refresh_token);
  return true;
}
export function clearTokens() {
  localStorage.removeItem(ACCESS);
  localStorage.removeItem(REFRESH);
  replaceSession();
}
