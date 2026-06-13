import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1, // don't hammer the API on a hard failure (e.g. 401/500)
      refetchOnWindowFocus: false,
    },
  },
});
