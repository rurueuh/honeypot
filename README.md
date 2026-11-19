# honeypot

A small HTTP server that looks like a forgotten admin panel. It answers the paths
internet scanners probe (`/.env`, `/.git/config`, `/wp-login.php`, `/phpmyadmin/`,
`/actuator/env`, router exploits, ...) with plausible fake content and logs every
request as one JSON line on stdout.

It runs behind a WAF in monitor mode, so the logs show what scanners really send and
the WAF decisions show which of those requests its rules catch.

Nothing behind it is real: no database, no outbound calls, no stored data. The fake
secrets are random values generated at startup.

## Run

```sh
go run .
docker build -t honeypot . && docker run --rm -p 8080:8080 honeypot
```

| Variable | Default | Meaning |
|---|---|---|
| `ADDR` | `:8080` | listen address |
| `MAX_BODY` | `65536` | request body bytes read |
| `LOG_BODY` | `4096` | request body bytes logged |
| `LOG_PER_IP_MINUTE` | `120` | requests logged per client and minute, the rest is counted in a `suppressed` line |

## Log line

```json
{"ts":"2026-11-19T20:00:00Z","event":"request","client":"203.0.113.7","method":"GET","host":"admin-portal.example","uri":"/.env","path":"/.env","proto":"HTTP/1.1","ua":"Mozilla/5.0","headers":{"Accept":"*/*"},"trap":"dotenv","status":200}
```

## License

MIT
