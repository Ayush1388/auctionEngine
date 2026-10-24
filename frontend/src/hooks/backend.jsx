// The backend (the real API or the demo engine) as a React context, plus who is signed in.
import { createContext, useContext, useEffect, useReducer, useState } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { getBackend } from "../../app/backend.js";
import { setBackend, startPulse, refreshWallet, startAlerts, resetAlerts } from "../lib/shell.js";

const Ctx = createContext(null);

export function BackendProvider({ children }) {
  const [be, setBe] = useState(null);
  useEffect(() => {
    let on = true;
    getBackend().then(b => {
      if (!on) return;
      setBackend(b); startPulse(); setBe(b);
    });
    return () => { on = false; };
  }, []);
  if (!be) return <div className="boot" role="status" aria-label="Connecting to the engine"><div className="skel" style={{ height: 64 }} /></div>;
  return <Ctx.Provider value={be}>{children}</Ctx.Provider>;
}

export const useBackend = () => useContext(Ctx);

/** Who is signed in, re-rendering when that changes. */
export function useAuth() {
  const be = useBackend();
  const [, bump] = useReducer(x => x + 1, 0);
  useEffect(() => be.onAuth(() => bump()), [be]);
  const s = be.session;
  return { user: s.user, id: s.id, isAdmin: s.isAdmin, signedIn: !!s.id };
}

/** Keeps the wallet readout and the alert engine in step with the signed-in person. */
export function useShellEffects() {
  const { id } = useAuth();
  useEffect(() => {
    resetAlerts();
    if (id) { refreshWallet(); startAlerts(); } else refreshWallet();
    const t = setInterval(refreshWallet, 30000);
    return () => clearInterval(t);
  }, [id]);
}

/** For pages that need a person: signed-out visitors are sent to sign in and come back. */
export function useRequireAuth() {
  const auth = useAuth();
  const loc = useLocation();
  const redirect = auth.signedIn ? null : <Navigate to={`/account?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />;
  return { ...auth, redirect };
}
