import React from "react";
import ReactDOM from "react-dom/client";
import { MantineProvider } from "@mantine/core";
import { Notifications } from "@mantine/notifications";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";

import "@mantine/core/styles.css";
import "@mantine/notifications/styles.css";
import "./build-stamp.css";

import App from "./App.tsx";
import { theme } from "./theme";

const queryClient = new QueryClient();
const BUILD_STAMP = "2026-05-20-1";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <MantineProvider theme={theme}>
          <Notifications position="top-right" />
          <App />
        </MantineProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>
);

document.documentElement.dataset.reviewdeskBuild = BUILD_STAMP;
