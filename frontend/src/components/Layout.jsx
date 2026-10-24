import { useEffect } from "react";
import { Outlet, useLocation } from "react-router-dom";
import AppBar from "./AppBar.jsx";
import Footer from "./Footer.jsx";
import EngineBadge from "./EngineBadge.jsx";
import Toast from "./Toast.jsx";
import { useShellEffects } from "../hooks/backend.jsx";

/** A route change starts at the top, or at the #anchor the link named. */
function ScrollManager() {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (hash) {
      const el = document.getElementById(hash.slice(1));
      if (el) { el.scrollIntoView(); return; }
    }
    window.scrollTo(0, 0);
  }, [pathname, hash]);
  return null;
}

function Extras() {
  useShellEffects();
  return <><ScrollManager /><EngineBadge /><Toast /></>;
}

/** Pages with the standard bar and footer. */
export function AppLayout() {
  return <><AppBar /><div className="route-fade"><Outlet /></div><Footer /><Extras /></>;
}

/** The home and event pages draw their own header over a photograph. */
export function BareLayout() {
  return <><Outlet /><Extras /></>;
}
