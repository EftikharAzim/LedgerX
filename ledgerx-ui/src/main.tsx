import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./app/queryClient";
import { AuthProvider } from "./app/auth";
import AppRouter from "./app/router";
import "./index.css";

const rootEl = document.getElementById("root");
if (!rootEl) {
  throw new Error("Root element not found: expected an element with id=\"root\"");
}

const root = createRoot(rootEl);
root.render(
  <React.StrictMode>
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <AppRouter />
      </QueryClientProvider>
    </AuthProvider>
  </React.StrictMode>
);

export {};