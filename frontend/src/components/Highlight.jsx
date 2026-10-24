/**
 * The server marks matches with <em>...</em>. Render exactly that and nothing else, as React
 * elements, so text that came from the network can never become markup.
 */
export function Highlight({ html }) {
  const parts = String(html || "").split(/(<\/?em>)/);
  const out = [];
  let on = false;
  parts.forEach((p, i) => {
    if (p === "<em>") on = true;
    else if (p === "</em>") on = false;
    else if (p) out.push(on ? <em key={i}>{p}</em> : p);
  });
  return <>{out}</>;
}
