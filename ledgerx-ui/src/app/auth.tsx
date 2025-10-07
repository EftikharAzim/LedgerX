import { createContext, useContext, useState, type ReactNode } from "react";

type AuthCtx = { token: string | null; login: (t: string) => void; logout: () => void; };
const Ctx = createContext<AuthCtx | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(localStorage.getItem("jwt"));
  const login = (t: string) => { localStorage.setItem("jwt", t); setToken(t); };
  const logout = () => { localStorage.removeItem("jwt"); setToken(null); };
  return <Ctx.Provider value={{ token, login, logout }}>{children}</Ctx.Provider>;
}
export const useAuth = () => {
  const v = useContext(Ctx);
  if (!v) throw new Error("AuthProvider missing");
  return v;
};