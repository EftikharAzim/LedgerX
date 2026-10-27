import { clearTokens, setTokens, type Tokens } from "../api/tokens.ts";
import { queryClient } from "./queryClient.ts";

function clearPrivateQueries() {
  // Cancel before removing queries so late responses cannot refill the cache.
  void queryClient.cancelQueries();
  queryClient.clear();
}

export function startSession(tokens: Tokens) {
  clearPrivateQueries();
  setTokens(tokens);
}

export function endSession() {
  clearPrivateQueries();
  clearTokens();
}
