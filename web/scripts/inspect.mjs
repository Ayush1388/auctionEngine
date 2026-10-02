// Opens the running app in Chromium and saves screenshots plus a report of
// console errors, failed requests and horizontal overflow.
//   node scripts/inspect.mjs [outDir] [path]
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const out = process.argv[2] ?? "inspect-out";
const path = process.argv[3] ?? "/";
const base = process.env.BASE_URL ?? "http://localhost:5173";
mkdirSync(out, { recursive: true });

const VIEWPORTS = [
  { name: "desktop-1366", width: 1366, height: 768 },
  { name: "desktop-1440", width: 1440, height: 900 },
  { name: "tablet-768", width: 768, height: 1024 },
  { name: "mobile-375", width: 375, height: 812, mobile: true },
];

// PW_CHROMIUM lets a pre-installed browser be used instead of the bundled download.
const browser = await chromium.launch(process.env.PW_CHROMIUM ? { executablePath: process.env.PW_CHROMIUM } : {});
for (const viewport of VIEWPORTS) {
  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    deviceScaleFactor: viewport.mobile ? 2 : 1,
    isMobile: Boolean(viewport.mobile),
    hasTouch: Boolean(viewport.mobile),
  });
  const page = await context.newPage();
  const problems = [];
  page.on("console", (message) => {
    if (message.type() === "error" || message.type() === "warning") problems.push(`console ${message.type()}: ${message.text()}`);
  });
  page.on("pageerror", (error) => problems.push(`page error: ${error.message}`));
  page.on("requestfailed", (request) => {
    // React StrictMode mounts twice in development and cancels the first requests.
    if (request.failure()?.errorText === "net::ERR_ABORTED") return;
    problems.push(`request failed: ${request.url()} ${request.failure()?.errorText}`);
  });
  page.on("response", (response) => {
    if (response.status() >= 400) problems.push(`HTTP ${response.status()}: ${response.url()}`);
  });

  await page.goto(base + path, { waitUntil: "networkidle" });
  await page.waitForTimeout(800);
  await page.screenshot({ path: `${out}/${viewport.name}-fold.png` });
  await page.screenshot({ path: `${out}/${viewport.name}-full.png`, fullPage: true });

  const overflow = await page.evaluate(() => {
    const width = document.documentElement.clientWidth;
    const wide = [...document.querySelectorAll("body *")]
      .filter((element) => element.getBoundingClientRect().right > width + 1)
      .slice(0, 5)
      .map((element) => `${element.tagName.toLowerCase()}.${String(element.className).slice(0, 60)}`);
    return { scrollWidth: document.documentElement.scrollWidth, clientWidth: width, wide };
  });
  console.log(`\n== ${viewport.name}`);
  console.log(`overflow: scrollWidth ${overflow.scrollWidth} vs ${overflow.clientWidth}`, overflow.wide);
  console.log(problems.length ? problems.join("\n") : "no console or network problems");
  await context.close();
}
await browser.close();
