import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "./styles/styles.css";
import "./styles/app.css";
import "./styles/pages.css";
import "./styles/ops.css";
import "./styles/lab.css";
import "./styles/lot.css";
import "./styles/event.css";
import "./styles/trace.css";
import "./styles/react.css";
import App from "./App.jsx";

createRoot(document.getElementById("root")).render(<BrowserRouter><App /></BrowserRouter>);
