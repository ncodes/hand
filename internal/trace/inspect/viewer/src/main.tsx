import { QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import { queryClient } from "./app/queryClient";
import "./styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("trace viewer root is missing");

createRoot(root).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
);
