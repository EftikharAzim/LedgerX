import { useSyncExternalStore, type ReactNode } from "react";
import { getAccess, getRefresh, subscribeAuth } from "../api/tokens";
import { logout as apiLogout } from "../api/endpoints";
import { AuthContext } from "./auth-context";
import { endSession, startSession } from "./auth-session";

export function AuthProvider({ children }: { children: ReactNode }) {
  const isAuthed = useSyncExternalStore(subscribeAuth, () => !!getAccess(), () => false);

  const logout = () => {
    // Best-effort server-side revocation; clear locally regardless.
    const refresh = getRefresh();
    endSession();
    if (refresh) void apiLogout(refresh).catch(() => {});
  };

  return <AuthContext.Provider value={{ isAuthed, login: startSession, logout }}>{children}</AuthContext.Provider>;
}
