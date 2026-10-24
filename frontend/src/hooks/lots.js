import { useEffect, useState } from "react";
import { useBackend } from "./backend.jsx";
import { liveLots } from "../../app/lots.js";

/** Every auction that carries a catalogue key, as Map(key -> lot); null until the backend has answered. */
export function useLiveLots() {
  const be = useBackend();
  const [lots, setLots] = useState(null);
  useEffect(() => {
    let on = true;
    liveLots(be).then(m => { if (on) setLots(m); }).catch(() => { if (on) setLots(new Map()); });
    return () => { on = false; };
  }, [be]);
  return lots;
}
