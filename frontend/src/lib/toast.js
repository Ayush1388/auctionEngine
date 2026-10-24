// A one-line message that fades after a moment. Any code can call toast(); the <Toast/> component shows it.
import { Emitter } from "../../app/util.js";

export const toastBus = new Emitter();
export const toast = (msg, ms = 2600) => toastBus.emit("toast", { msg: String(msg), ms });
