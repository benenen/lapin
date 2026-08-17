#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
base_url=${LAPIN_WEB_BASE_URL:-http://127.0.0.1:5173}
session="lapin-whiteboard-scroll-$$"
config=${PLAYWRIGHT_CLI_CONFIG:-$repo_root/scripts/playwright-cli.config.json}
cli_bin=${PLAYWRIGHT_CLI_BIN:-}

if [[ ! $base_url =~ ^http://(127\.0\.0\.1|localhost|\[::1\])(:[0-9]+)?/?$ ]]; then
  echo 'LAPIN_WEB_BASE_URL must be a loopback HTTP origin' >&2
  exit 2
fi
if [[ -z $cli_bin && -x $repo_root/web/node_modules/.bin/playwright-cli ]]; then
  cli_bin=$repo_root/web/node_modules/.bin/playwright-cli
fi
if [[ -z $cli_bin ]]; then
  cli_bin=$(command -v playwright-cli || true)
fi
if [[ -z $cli_bin || ! -x $cli_bin ]]; then
  echo 'playwright-cli 0.1.18 must be installed locally; runtime downloads are disabled' >&2
  exit 2
fi

run_cli() {
  PLAYWRIGHT_CLI_SESSION="$session" "$cli_bin" "$@"
}

cleanup() {
  run_cli close >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP

run_cli open "$base_url" --config "$config" >/dev/null
run_cli run-code 'async page => {
  await page.evaluate(async () => {
    document.body.style.margin = "0";
    document.body.innerHTML = `<div style="height:300px"></div><div style="position:relative;width:1000px;height:2200px"><div id="whiteboard-e2e" class="excalidraw-host" aria-hidden="false" style="visibility:visible;pointer-events:auto"></div></div><div style="height:1000px"></div>`;
    const module = await import("/src/excalidrawBridge.ts");
    const host = document.querySelector("#whiteboard-e2e");
    window.__lapinWhiteboardE2E = module.mountExcalidraw(host, { width: 1000, height: 2200, topInset: 0 });
  });
  for (let attempt = 0; attempt < 100; attempt++) {
    if (await page.evaluate(() => window.__lapinWhiteboardE2E?.isReady())) break;
    await page.waitForTimeout(50);
  }
  const ready = await page.evaluate(() => window.__lapinWhiteboardE2E?.isReady());
  if (!ready) throw new Error("whiteboard did not become ready");
  const touchAction = await page.locator("#whiteboard-e2e").evaluate(element => getComputedStyle(element).touchAction);
  if (touchAction !== "pan-y") throw new Error(`unexpected touch-action ${touchAction}`);
	const scrollHeight = await page.evaluate(() => document.documentElement.scrollHeight);
	if (scrollHeight < 3000) throw new Error(`test page is not scrollable: ${scrollHeight}`);
  await page.mouse.move(400, 600);
  await page.mouse.wheel(0, 700);
  const scrolled = await page.evaluate(() => window.scrollY);
  if (scrolled < 600) throw new Error(`plain wheel did not scroll the page: ${scrolled}`);
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, 400);
  await page.keyboard.up("Control");
  const afterZoomGesture = await page.evaluate(() => window.scrollY);
  if (afterZoomGesture !== scrolled) throw new Error("modified wheel changed the document viewport");
  await page.mouse.move(350, 400);
  await page.mouse.down();
  await page.mouse.move(620, 560, { steps: 12 });
  await page.mouse.up();
  for (let attempt = 0; attempt < 40; attempt++) {
    const count = await page.evaluate(() => window.__lapinWhiteboardE2E?.getDocument().elements.length ?? 0);
    if (count > 0) return { scrollY: scrolled, elements: count, touchAction };
    await page.waitForTimeout(25);
  }
  throw new Error("first stroke after document scroll was not recorded");
}'

echo 'whiteboard browser scroll test passed'
