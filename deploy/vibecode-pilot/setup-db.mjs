// Deploy step `preStart` on a dedicated VM (runs as root): makes sure the role
// and database in DATABASE_URL exist, runs the migrations and records this
// deploy, so the report can show the database outlives a redeploy. The result
// goes to pilot-setup.json for the app, which runs as an unprivileged user.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const steps = [];

// The deploy writes the app's environment to .env before preStart but runs
// preStart without it; only the service loads it (EnvironmentFile).
try {
  process.loadEnvFile(join(root, ".env"));
} catch {
  // No file: the variables may come from the environment.
}

function run(label, command, args, input) {
  const result = spawnSync(command, args, { cwd: root, encoding: "utf8", input, timeout: 120000 });
  const step = {
    label,
    status: result.status,
    error: result.error ? result.error.code || result.error.message : null,
    stdout: (result.stdout || "").trim().slice(-2000),
    stderr: (result.stderr || "").trim().slice(-2000),
  };
  steps.push(step);
  console.log(JSON.stringify(step));
  return step;
}

function finish(ok, error) {
  writeFileSync(join(root, "pilot-setup.json"), JSON.stringify({ ok, error, at: new Date().toISOString(), steps }, null, 2));
  if (!ok) console.error(error);
  // A failed setup still lets the app start, so the report can show why.
  process.exit(0);
}

for (const name of ["vibeprobe", "server", "goose"]) {
  const file = join(root, "bin", name);
  if (existsSync(file)) chmodSync(file, 0o755);
}

let url;
try {
  url = new URL(process.env.DATABASE_URL || "");
} catch {
  finish(false, "DATABASE_URL is not set or not a URL");
}
const user = decodeURIComponent(url.username);
const password = decodeURIComponent(url.password);
const database = url.pathname.slice(1);
if (!/^[a-z_][a-z0-9_]*$/.test(user) || !/^[a-z_][a-z0-9_]*$/.test(database) || !password) {
  finish(false, "DATABASE_URL needs a lowercase user and database name and a password");
}

run("clusters", "pg_lsclusters", []);
run("start postgres", "systemctl", ["start", "postgresql"]);

// psql reads variables from stdin scripts, not from -c, so the password is
// passed as :'password' and quoted by psql.
const asPostgres = (label, sql) =>
  run(label, "runuser", ["-u", "postgres", "--", "psql", "-X", "-v", "ON_ERROR_STOP=1", "-v", `password=${password}`, "-tA"], sql);

const roleExists = asPostgres("role exists?", `SELECT 1 FROM pg_roles WHERE rolname = '${user}';`).stdout === "1";
const role = asPostgres(
  roleExists ? "update role password" : "create role",
  `${roleExists ? "ALTER" : "CREATE"} ROLE ${user} LOGIN PASSWORD :'password';`,
);
if (role.status !== 0) finish(false, "could not create the role");

const databaseExists = asPostgres("database exists?", `SELECT 1 FROM pg_database WHERE datname = '${database}';`).stdout === "1";
if (!databaseExists) {
  const created = asPostgres("create database", `CREATE DATABASE ${database} OWNER ${user};`);
  if (created.status !== 0) finish(false, "could not create the database");
}

const migrated = run("goose up", "./bin/goose", ["-dir", "db/migrations", "postgres", url.toString(), "up"]);
if (migrated.status !== 0) finish(false, "migrations failed");

run(
  "record deploy",
  "psql",
  [url.toString(), "-X", "-v", "ON_ERROR_STOP=1", "-tA"],
  "CREATE TABLE IF NOT EXISTS pilot_deploys (id serial PRIMARY KEY, deployed_at timestamptz NOT NULL DEFAULT now());\nINSERT INTO pilot_deploys DEFAULT VALUES;",
);

finish(true, null);
