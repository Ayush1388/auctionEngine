import { Link } from "react-router-dom";
import { useAuth, useBackend } from "../hooks/backend.jsx";

/** Operator-only pages: shows a sign-in prompt instead of the page when the visitor is not an operator. */
export default function OperatorGate({ what, next, children }) {
  const be = useBackend();
  const { signedIn, isAdmin } = useAuth();
  if (be.unreachable) {
    return (
      <div className="empty"><h3>The API is not answering</h3>
        <p>Nothing is listening at {be.base}. Start it, or use the demo engine, which needs no server.</p>
        <a className="btn" href="?engine=sim">Use the demo engine</a></div>
    );
  }
  if (isAdmin) return children;
  return (
    <div className="empty"><h3>Operators only</h3>
      <p>{what} {signedIn ? "This account is not an operator." : "Sign in as the operator account to use it."}{" "}
        {be.kind === "sim" ? "In the demo, the operator is admin@marque.test with the password on the sign-in page." : "Make an account an operator with: go run ./cmd/admin promote <email>"}</p>
      <Link className="btn" to={`/account?next=${encodeURIComponent(next)}`}>{signedIn ? "Switch account" : "Sign in"}</Link></div>
  );
}
