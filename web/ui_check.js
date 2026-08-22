// Verifies the page's own render logic against a live snapshot.
// Pulls the functions straight out of index.html so this cannot drift from
// what the browser actually runs.  Usage: node web/ui_check.js [port]
const fs = require("fs");
const path = require("path");
const http = require("http");

const port = process.argv[2] || "8100";
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const script = html.match(/<script>([\s\S]*)<\/script>/)[1];

// Minimal DOM so the page script can be evaluated headlessly.
const els = {};
const mkEl = () => ({
  textContent: "", innerHTML: "", value: "", style: {},
  classList: { add() {}, remove() {} }, dataset: {}, disabled: false,
  addEventListener() {},
});
const doc = {
  getElementById: (id) => (els[id] ||= mkEl()),
  querySelectorAll: () => [],
};
globalThis.document = doc;
globalThis.console = console;
// The page's bootstrap IIFE fires on load; stub it out so the harness controls
// the snapshot. (It also tolerates rejection here — its try/catch handles it.)
globalThis.fetch = () => Promise.resolve({ json: () => Promise.resolve({}) });
globalThis.setInterval = () => 0;

// Evaluate the page script, then hand back the internals we want to exercise.
const api = new Function(
  "document", "fetch", "setInterval",
  script + "\n;return { render, usd, pct, ago, esc, safeURL, setSnap: (s) => { snap = s; } };"
)(doc, globalThis.fetch, globalThis.setInterval);

let fail = 0;
const ok = (cond, msg) => { if (!cond) { fail++; console.log("  FAIL " + msg); } };

// --- pure formatters --------------------------------------------------------
console.log("formatters:");
ok(api.usd(49274964843) === "$49.27B", "usd billions -> " + api.usd(49274964843));
ok(api.usd(1553716335239) === "$1.55T", "usd trillions -> " + api.usd(1553716335239));
ok(api.usd(688279174) === "$688.28M", "usd millions -> " + api.usd(688279174));
ok(api.usd(0) === "—", "usd zero -> " + api.usd(0));
ok(api.usd(undefined) === "—", "usd undefined -> " + api.usd(undefined));
ok(api.pct(3.14522659).includes("+3.15"), "pct positive -> " + api.pct(3.14));
ok(api.pct(-0.09).includes("down"), "pct negative class -> " + api.pct(-0.09));
console.log("  usd/pct OK");

// --- escaping ---------------------------------------------------------------
console.log("escaping:");
ok(api.esc('<img src=x onerror=alert(1)>') === "&lt;img src=x onerror=alert(1)&gt;",
   "esc angle brackets -> " + api.esc("<img>"));
ok(api.esc('a"b\'c&d') === "a&quot;b&#39;c&amp;d", "esc quotes/amp -> " + api.esc('a"b\'c&d'));
ok(api.safeURL("https://coinmarketcap.com/currencies/bitcoin/") !== "", "allows cmc url");
ok(api.safeURL("javascript:alert(1)") === "", "blocks javascript: url");
ok(api.safeURL("https://evil.example/x") === "", "blocks foreign host");
console.log("  esc/safeURL OK");

// --- render against the live snapshot --------------------------------------
http.get({ host: "localhost", port, path: "/api/assets" }, (res) => {
  let body = "";
  res.on("data", (d) => (body += d));
  res.on("end", () => {
    const snap = JSON.parse(body);
    api.setSnap(snap);
    api.render();

    const rows = els.rows.innerHTML;
    const rowCount = (rows.match(/<tr>/g) || []).length;
    console.log("render against live snapshot:");
    console.log("  assets in snapshot :", snap.assets.length);
    console.log("  <tr> rendered      :", rowCount);
    ok(rowCount === snap.assets.length, "row count matches asset count");
    ok(!/undefined|NaN/.test(rows), "no undefined/NaN leaked into markup");
    ok(/href="https:\/\/coinmarketcap\.com\//.test(rows), "coin links present");
    ok(els["s-count"].textContent === snap.assets.length, "count tile");
    ok(els["s-src"].textContent === snap.source, "source tile -> " + els["s-src"].textContent);
    ok(els["s-stable"].textContent === snap.stablesExcluded, "stablecoin tile");
    ok(els["s-vol"].textContent.startsWith("$"), "volume tile -> " + els["s-vol"].textContent);
    ok(els.banner.innerHTML === "", "no error banner");

    // Widest volume bar must belong to the top asset.
    const widths = [...rows.matchAll(/width:([\d.]+)%/g)].map((m) => +m[1]);
    ok(widths[0] === 100, "top row bar is full width -> " + widths[0]);
    ok(widths.every((w, i) => i === 0 || w <= widths[i - 1]), "bars descend monotonically");

    // A hostile coin name must not produce live markup.
    api.setSnap({
      ...snap,
      assets: [{ ...snap.assets[0], name: '<img src=x onerror=alert(1)>', url: "javascript:alert(1)" }],
    });
    api.render();
    const evil = els.rows.innerHTML;
    ok(!/<img/.test(evil), "hostile name not rendered as a tag");
    ok(!/javascript:/.test(evil), "javascript: url stripped");
    console.log("  xss payload neutralised");

    console.log(fail === 0 ? "\nALL UI CHECKS PASSED" : `\n${fail} UI CHECK(S) FAILED`);
    process.exit(fail === 0 ? 0 : 1);
  });
}).on("error", (e) => {
  console.log("could not reach server on port " + port + ": " + e.message);
  process.exit(1);
});
