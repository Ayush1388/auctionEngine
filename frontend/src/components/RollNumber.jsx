// A number that rolls to its new value when it really changes (a bid landing), and sits still
// otherwise. Motion that answers something that happened: hierarchy and feedback, never decoration.
import { useLayoutEffect, useRef } from "react";
import gsap from "gsap";

const reduce = () => matchMedia("(prefers-reduced-motion: reduce)").matches;

export default function RollNumber({ value, format, className, id, as: Tag = "strong" }) {
  const el = useRef(null);
  const prev = useRef(undefined);
  const tween = useRef(null);

  useLayoutEffect(() => {
    const node = el.current;
    const from = prev.current;
    prev.current = value;
    tween.current?.kill();
    if (from === undefined || from === value || value == null || from == null || reduce()) { node.textContent = format(value); return undefined; }
    const o = { v: from };
    tween.current = gsap.to(o, { v: value, duration: 0.7, ease: "power2.out", onUpdate: () => { node.textContent = format(Math.round(o.v)); }, onComplete: () => { node.textContent = format(value); } });
    gsap.fromTo(node, { y: -6 }, { y: 0, duration: 0.5, ease: "power3.out" });
    return undefined;
  }, [value]);   // eslint-disable-line react-hooks/exhaustive-deps

  useLayoutEffect(() => () => tween.current?.kill(), []);
  return <Tag className={className} id={id} ref={el} />;
}
