// VibeCode pilot (#171): starts the Go probe behind a Node wrapper and reports
// what the platform allows. No dependencies: the platform runs `npm install`
// and `npm start` only.
import { spawn, spawnSync } from "node:child_process";
import { chmodSync, existsSync, readFileSync, readdirSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import http from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const port = Number(process.env.PORT || 3000);
const goHost = "127.0.0.1";
const goPort = 8080;
const startedAt = new Date().toISOString();

// Install steps run as root and the app as an unprivileged user, so the mode
// set at upload may not survive; set it here and record what happened.
const chmodResults = {};
for (const name of ["vibeprobe", "server", "goose"]) {
  const file = join(root, "bin", name);
  if (!existsSync(file)) {
    chmodResults[name] = "missing";
    continue;
  }
  try {
    chmodSync(file, 0o755);
    chmodResults[name] = "755";
  } catch (error) {
    chmodResults[name] = `chmod failed: ${error.code || error.message}`;
  }
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: root, encoding: "utf8", timeout: 5000, ...options });
  return {
    command: [command, ...args].join(" "),
    status: result.status,
    signal: result.signal,
    error: result.error ? result.error.code || result.error.message : null,
    // The end: that's where long outputs (goose) say how they finished.
    stdout: (result.stdout || "").trim().slice(-2000),
    stderr: (result.stderr || "").trim().slice(-2000),
  };
}

// The pilot writes to the database (migrations, the API's default blocks), so it
// does that only to one whose name ends in "pilot", never to the real one.
function pilotDatabase() {
  try {
    const url = new URL(process.env.DATABASE_URL || "");
    const name = url.pathname.slice(1);
    return { url: url.toString(), name, writable: /pilot$/.test(name) };
  } catch {
    return null;
  }
}
const database = pilotDatabase();

// Through goose's own variables, so the URL stays out of the report.
const migrations = database?.writable
  ? run("./bin/goose", ["up"], {
      timeout: 180000,
      env: {
        PATH: process.env.PATH,
        GOOSE_DRIVER: "postgres",
        GOOSE_DBSTRING: database.url,
        GOOSE_MIGRATION_DIR: "db/migrations",
      },
    })
  : `skipped: ${database ? `database "${database.name}" does not end in "pilot"` : "DATABASE_URL is not set"}`;

// EACCES here means noexec (or wrong mode); ENOENT on an existing file means a
// missing loader, i.e. the binary is not static.
const execChecks = {
  probeVersion: run("./bin/vibeprobe", ["--version"]),
  migrations,
  // The real API, briefly. Against the pilot database it should keep running
  // until the timeout kills it (signal SIGTERM, "API server listening");
  // without one it exits on the missing database, which still proves it runs.
  // PORT is moved off the wrapper's and the probe's.
  serverStart: run("./bin/server", [], {
    timeout: 8000,
    env: { PATH: process.env.PATH, DATABASE_URL: database?.writable ? database.url : "", PORT: "8090" },
  }),
};

function readSetup() {
  try {
    return JSON.parse(readFileSync(join(root, "pilot-setup.json"), "utf8"));
  } catch (error) {
    return `no setup result: ${error.code || error.message}`;
  }
}

// Read-only: connection time, versions and query round trips, measured by the
// probe with pgx (there is no psql on Galaxy).
function databaseCheck() {
  const result = run("./bin/vibeprobe", ["--db"], {
    timeout: 60000,
    env: { PATH: process.env.PATH, DATABASE_URL: process.env.DATABASE_URL || "" },
  });
  try {
    return JSON.parse(result.stdout);
  } catch {
    return { ok: false, ...result };
  }
}

function dataVolumeCheck() {
  const file = "/data/.vibecode-pilot-write-test";
  try {
    writeFileSync(file, startedAt);
    unlinkSync(file);
    return "writable";
  } catch (error) {
    return `not writable: ${error.code || error.message}`;
  }
}

function binListing() {
  try {
    return readdirSync(join(root, "bin")).map((name) => {
      const stat = statSync(join(root, "bin", name));
      return `${(stat.mode & 0o777).toString(8)} ${stat.uid}:${stat.gid} ${stat.size} ${name}`;
    });
  } catch (error) {
    return [`unreadable: ${error.code || error.message}`];
  }
}

const environmentChecks = {
  node: process.version,
  uid: process.getuid?.(),
  gid: process.getgid?.(),
  cwd: process.cwd(),
  root,
  mounts: run("sh", ["-c", "mount | grep -E ' /opt| /data' || true"]).stdout,
  bin: binListing(),
  chmod: chmodResults,
  dataVolume: dataVolumeCheck(),
  setup: readSetup(),
  // Names only: values may be secrets.
  env: Object.fromEntries(
    ["PORT", "DATABASE_URL", "BITRIX_API_BASE_URL", "BITRIX_API_KEY", "BITRIX_WEBHOOK_URL", "PILOT_AI_MODEL"].map(
      (name) => [name, Boolean(process.env[name])],
    ),
  ),
};

let probe = null;
let probeLog = [];
let stopping = false;

function startProbe() {
  probe = spawn("./bin/vibeprobe", ["--addr", `${goHost}:${goPort}`], { cwd: root, stdio: ["ignore", "pipe", "pipe"] });
  const keep = (chunk) => {
    probeLog = [...probeLog, ...chunk.toString().split("\n").filter(Boolean)].slice(-50);
  };
  probe.stdout.on("data", keep);
  probe.stderr.on("data", keep);
  probe.on("error", (error) => keep(Buffer.from(`spawn error: ${error.code || error.message}`)));
  probe.on("exit", (code, signal) => {
    keep(Buffer.from(`exited: code ${code}, signal ${signal}`));
    if (!stopping) setTimeout(startProbe, 1000);
  });
}
startProbe();

// The session token stays out of the report: its first characters show the kind.
function vibeHeaders(request) {
  return Object.fromEntries(
    Object.entries(request.headers)
      .filter(([name]) => name.startsWith("x-vibe-"))
      .map(([name, value]) => [name, name === "x-vibe-authorization" ? `${value.slice(0, 20)}… (${value.length} chars)` : value]),
  );
}

function sendJSON(response, status, body) {
  response.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" });
  response.end(JSON.stringify(body, null, 2));
}

function proxyToGo(request, response) {
  const upstream = http.request(
    { host: goHost, port: goPort, method: request.method, path: request.url, headers: request.headers },
    (upstreamResponse) => {
      response.writeHead(upstreamResponse.statusCode || 502, upstreamResponse.headers);
      // Send headers and each chunk at once: no compression, no buffering.
      response.flushHeaders();
      response.socket?.setNoDelay(true);
      upstreamResponse.pipe(response);
    },
  );
  upstream.on("error", (error) => sendJSON(response, 502, { error: `probe unreachable: ${error.code || error.message}` }));
  // The response, not the request: a request closes once its body is read.
  response.on("close", () => upstream.destroy());
  request.pipe(upstream);
}

// The same stream from Node itself, to tell the gateway's buffering from the
// proxy's if the Go stream arrives in one burst.
function nodeTicks(request, response) {
  response.writeHead(200, {
    "Content-Type": "text/event-stream",
    "Cache-Control": "no-cache, no-transform",
    "X-Accel-Buffering": "no",
  });
  response.flushHeaders();
  response.socket?.setNoDelay(true);
  let n = 0;
  const timer = setInterval(() => {
    n += 1;
    response.write(`event: tick\ndata: ${JSON.stringify({ n, sentAt: Date.now() })}\n\n`);
    if (n === 10) {
      clearInterval(timer);
      response.end("event: done\ndata: {}\n\n");
    }
  }, 1000);
  response.on("close", () => clearInterval(timer));
}

async function timed(label, action) {
  const started = Date.now();
  try {
    return { label, ...(await action()), ms: Date.now() - started };
  } catch (error) {
    return { label, ok: false, error: error.cause?.code || error.message, ms: Date.now() - started };
  }
}

async function readResponse(apiResponse) {
  const text = await apiResponse.text();
  try {
    return JSON.parse(text);
  } catch {
    return text.slice(0, 2000);
  }
}

function checkAI() {
  return timed("ai", async () => {
    const base = (process.env.BITRIX_API_BASE_URL || "").replace(/\/+$/, "");
    if (!base || !process.env.BITRIX_API_KEY) return { ok: false, error: "BITRIX_API_BASE_URL or BITRIX_API_KEY is not set" };
    const model = process.env.PILOT_AI_MODEL || "bitrix/bitrixgpt-5.5";
    const apiResponse = await fetch(`${base}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Api-Key": process.env.BITRIX_API_KEY },
      body: JSON.stringify({
        model,
        messages: [
          { role: "system", content: "You check email subject lines." },
          { role: "user", content: 'Rate the subject line "Your order has shipped" for clarity from 1 to 5.' },
        ],
        response_format: {
          type: "json_schema",
          json_schema: {
            name: "subject_rating",
            strict: true,
            schema: {
              type: "object",
              properties: { score: { type: "integer" }, reason: { type: "string" } },
              required: ["score", "reason"],
              additionalProperties: false,
            },
          },
        },
      }),
      signal: AbortSignal.timeout(60000),
    });
    const body = await readResponse(apiResponse);
    const content = body?.choices?.[0]?.message?.content;
    let parsed = null;
    try {
      parsed = content ? JSON.parse(content) : null;
    } catch {
      parsed = null;
    }
    return {
      ok: apiResponse.ok && parsed !== null,
      status: apiResponse.status,
      model: body?.model || model,
      parsed,
      body: parsed ? undefined : body,
    };
  });
}

// What the platform tells about the signed-in user: field names only, and
// whether an email is there, since #172 links users by it.
function checkIdentity(request) {
  return timed("identity", async () => {
    const base = (process.env.BITRIX_API_BASE_URL || "").replace(/\/+$/, "");
    const userId = request.headers["x-vibe-user-id"];
    if (!base || !process.env.BITRIX_API_KEY) return { ok: false, error: "BITRIX_API_BASE_URL or BITRIX_API_KEY is not set" };
    if (!userId) return { ok: false, error: "no X-Vibe-User-Id: open the app through the gateway" };
    const describe = async (path, headers) => {
      const started = Date.now();
      const apiResponse = await fetch(`${base}${path}`, {
        headers: { "X-Api-Key": process.env.BITRIX_API_KEY, ...headers },
        signal: AbortSignal.timeout(120000),
      });
      const body = await readResponse(apiResponse);
      const data = body?.data ?? body;
      const text = JSON.stringify(body);
      return {
        status: apiResponse.status,
        ms: Date.now() - started,
        fields: data && typeof data === "object" ? Object.keys(data) : null,
        hasEmail: /"[^"]*email[^"]*"\s*:\s*"[^"]+@/i.test(text),
        error: apiResponse.ok ? undefined : body?.error ?? text.slice(0, 500),
      };
    };
    const user = await describe(`/users/${encodeURIComponent(userId)}`);
    const session = request.headers["x-vibe-authorization"];
    const me = session ? await describe("/me", { Authorization: session }) : "no X-Vibe-Authorization";
    return { ok: user.status === 200 && user.hasEmail, user, me };
  });
}

function checkCRM() {
  return timed("crm", async () => {
    const webhook = (process.env.BITRIX_WEBHOOK_URL || "").trim().replace(/\/+$/, "");
    if (!webhook) return { ok: false, error: "BITRIX_WEBHOOK_URL is not set" };
    const apiResponse = await fetch(`${webhook}/crm.deal.list.json`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ select: ["ID"], order: { ID: "DESC" } }),
      signal: AbortSignal.timeout(30000),
    });
    const body = await readResponse(apiResponse);
    // Counts only: deal data stays out of the report.
    return {
      ok: apiResponse.ok && Array.isArray(body?.result),
      status: apiResponse.status,
      dealsOnPage: Array.isArray(body?.result) ? body.result.length : null,
      total: body?.total ?? null,
      error: body?.error ? `${body.error}: ${body.error_description || ""}` : undefined,
    };
  });
}

const server = http.createServer(async (request, response) => {
  const path = new URL(request.url, "http://localhost").pathname;
  if (path === "/api" || path.startsWith("/api/") || path === "/health") return proxyToGo(request, response);
  if (path === "/pilot/report") {
    return sendJSON(response, 200, { startedAt, execChecks, environmentChecks, probeLog, vibeHeaders: vibeHeaders(request) });
  }
  if (path === "/pilot/sse-node") return nodeTicks(request, response);
  if (path === "/pilot/ai") return sendJSON(response, 200, await checkAI());
  if (path === "/pilot/crm") return sendJSON(response, 200, await checkCRM());
  if (path === "/pilot/identity") return sendJSON(response, 200, await checkIdentity(request));
  if (path === "/pilot/db") return sendJSON(response, 200, databaseCheck());
  if (path === "/" || path === "/index.html") {
    response.writeHead(200, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" });
    return response.end(readFileSync(join(root, "index.html")));
  }
  sendJSON(response, 404, { error: "not found" });
});

server.listen(port, () => console.log(`vibecode pilot on :${port}`));

process.on("SIGTERM", () => {
  stopping = true;
  probe?.kill("SIGTERM");
  server.close(() => process.exit(0));
});
