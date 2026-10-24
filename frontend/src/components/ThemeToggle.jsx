import { useState } from "react";
import { local } from "../../app/util.js";

const isDark = () => document.documentElement.getAttribute("data-theme") === "dark";

/** The Noir switch. One handler, one source of truth: the data-theme attribute on <html>. */
export default function ThemeToggle() {
  const [dark, setDark] = useState(isDark);
  const flip = () => {
    const next = !isDark();
    if (next) document.documentElement.setAttribute("data-theme", "dark"); else document.documentElement.removeAttribute("data-theme");
    local.set("marque-theme", next ? "dark" : "light");
    setDark(next);
  };
  return (
    <button className="mode" id="modeToggle" role="switch" aria-checked={dark} aria-label="Noir mode" onClick={flip}>
      <span className="mono mode-label">Noir</span>
      <span className="mode-track"><span className="mode-knob" /></span>
    </button>
  );
}
