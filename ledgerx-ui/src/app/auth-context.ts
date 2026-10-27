import { createContext, useContext } from "react";
import type { Tokens } from "../api/tokens";

type AuthContextValue = {
  isAuthed: boolean;
  login: (tokens: Tokens) => void;
  logout: () => void;
};

export const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("AuthProvider missing");
  return value;
}
