// Deploy step `preStart` (root, before the app starts): makes sure the role and
// database in DATABASE_URL exist, then runs the migrations. Any failure fails
// the deploy.
import { spawnSync } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));

// The deploy writes the app's environment to .env before preStart but runs
// preStart without it; only the service loads it (EnvironmentFile).
process.loadEnvFile(join(root, ".env"));

function fail(message) {
  console.error(`prestart: ${message}`);
  process.exit(1);
}

function run(command, args, input) {
  const result = spawnSync(command, args, { cwd: root, encoding: "utf8", input, timeout: 120000 });
  if (result.stdout?.trim()) console.log(result.stdout.trim());
  if (result.stderr?.trim()) console.error(result.stderr.trim());
  return result;
}

let url;
try {
  url = new URL(process.env.DATABASE_URL || "");
} catch {
  fail("DATABASE_URL is not set or not a URL");
}
const user = decodeURIComponent(url.username);
const password = decodeURIComponent(url.password);
const database = url.pathname.slice(1);
if (!/^[a-z_][a-z0-9_]*$/.test(user) || !/^[a-z_][a-z0-9_]*$/.test(database) || !password) {
  fail("DATABASE_URL needs a lowercase user and database name and a password");
}

if (run("systemctl", ["start", "postgresql"]).status !== 0) fail("could not start PostgreSQL");

// psql reads variables in scripts from stdin, not in -c, so the password goes
// in as :'password' and psql quotes it.
const asPostgres = (sql) =>
  run("runuser", ["-u", "postgres", "--", "psql", "-X", "-v", "ON_ERROR_STOP=1", "-v", `password=${password}`, "-tA"], sql);

const roleExists = asPostgres(`SELECT 1 FROM pg_roles WHERE rolname = '${user}';`).stdout.trim() === "1";
// Setting the password each time keeps the role in step with DATABASE_URL.
if (asPostgres(`${roleExists ? "ALTER" : "CREATE"} ROLE ${user} LOGIN PASSWORD :'password';`).status !== 0) {
  fail("could not create the role");
}
if (asPostgres(`SELECT 1 FROM pg_database WHERE datname = '${database}';`).stdout.trim() !== "1") {
  if (asPostgres(`CREATE DATABASE ${database} OWNER ${user};`).status !== 0) fail("could not create the database");
}

// Retries as in apps/api/entrypoint.sh: PostgreSQL may still be starting.
// goose takes the URL from its own variables, so it stays out of the log.
const gooseEnv = {
  PATH: process.env.PATH,
  GOOSE_DRIVER: "postgres",
  GOOSE_DBSTRING: url.toString(),
  GOOSE_MIGRATION_DIR: join(root, "db/migrations"),
};
for (let attempt = 1; ; attempt++) {
  const result = spawnSync(join(root, "bin/goose"), ["up"], { cwd: root, encoding: "utf8", env: gooseEnv, stdio: "inherit" });
  if (result.status === 0) break;
  if (attempt === 12) fail(`migrations failed after ${attempt} attempts`);
  console.error(`prestart: migrations failed on attempt ${attempt}; retrying in 5 seconds`);
  await sleep(5000);
}
