import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router";
import { DiscoverPage } from "../features/discover/DiscoverPage";
import { AppShell } from "./AppShell";
import { NotBuiltYet } from "./NotBuiltYet";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1 } },
});

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<AppShell />}>
            <Route index element={<DiscoverPage />} />
            <Route path="*" element={<NotBuiltYet />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
