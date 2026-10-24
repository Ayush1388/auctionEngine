import { useEffect, useRef, useState } from "react";
import { toastBus } from "../lib/toast.js";

export default function Toast() {
  const [t, setT] = useState({ msg: "", show: false });
  const timer = useRef(0);
  useEffect(() => toastBus.on("toast", ({ msg, ms }) => {
    setT({ msg, show: true });
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setT(s => ({ ...s, show: false })), ms);
  }), []);
  return <div className={"toast" + (t.show ? " show" : "")} id="toast" role="status" aria-live="polite">{t.msg}</div>;
}
