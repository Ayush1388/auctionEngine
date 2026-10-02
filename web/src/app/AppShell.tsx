import { useEffect, useRef } from "react";
import { Outlet, useLocation } from "react-router";
import { BottomTabs } from "./BottomTabs";
import { Footer } from "./Footer";
import { Navbar } from "./Navbar";

export function AppShell() {
  const { pathname } = useLocation();
  const main = useRef<HTMLElement>(null);
  const firstRender = useRef(true);

  // After a route change, start at the top and move focus to the new page
  // so keyboard and screen-reader users are not left in the old one.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    window.scrollTo(0, 0);
    main.current?.focus({ preventScroll: true });
  }, [pathname]);

  return (
    <div className="flex min-h-dvh flex-col">
      <a
        href="#main"
        className="meta sr-only z-50 rounded-control bg-action px-4 py-3 text-16 text-on-action focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        Skip to main content
      </a>
      <Navbar />
      <main ref={main} id="main" tabIndex={-1} className="flex-1 outline-none">
        <Outlet />
      </main>
      <Footer />
      {/* Room for the fixed bottom tabs on mobile. */}
      <div aria-hidden="true" className="h-[calc(3.5rem+env(safe-area-inset-bottom))] md:hidden" />
      <BottomTabs />
    </div>
  );
}
