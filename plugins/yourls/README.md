# YOURLS uploader IP plugin

This optional plugin lets YOURLS record the uploader's address forwarded by AppBox middleware 3.1.0 or later. Install it in YOURLS. The middleware remains a Go-only service and needs no PHP runtime.

## Install

1. Copy the `appbox-middleware-ip` directory into YOURLS's persistent `user/plugins/` directory. In the standard Docker image, the resulting file is `/var/www/html/user/plugins/appbox-middleware-ip/plugin.php`.
2. Set `APPBOX_MIDDLEWARE_TRUSTED_IPS` in the **YOURLS container's environment** to the middleware container's exact addresses on their shared network. Recreate the YOURLS container after changing its environment, retaining its existing data volumes.
3. Open **YOURLS admin → Manage Plugins** and activate **AppBox Middleware Client IP**.
4. Create a short link for a new target URL and check its saved IP. Existing records keep their original IP, including when YOURLS returns an already-created short link.

Example environment entry, using documentation-only addresses:

```yaml
environment:
  APPBOX_MIDDLEWARE_TRUSTED_IPS: "192.0.2.10,2001:db8::10"
```

Replace both example addresses with the middleware addresses shown for the shared network in your deployment platform or `docker network inspect`. Keep the setting available to PHP. Do not use the uploader's address or the reverse proxy's address here.

## Configuration

| Setting | Where | Default | Accepted values |
|---|---|---|---|
| `APPBOX_MIDDLEWARE_TRUSTED_IPS` | YOURLS | Empty, trusts no forwarded addresses | Comma-separated exact IPv4 or IPv6 addresses of middleware peers. Whitespace is trimmed. CIDR ranges and hostnames are rejected. |
| `TRUSTED_PROXY_CIDRS` | Middleware | Empty, retains count-based trust | IPs or CIDRs of the immediate reverse proxies connecting to middleware. This separate setting does not configure YOURLS. |
| `TRUSTED_PROXY_COUNT` | Middleware | `1` | Number of trusted forwarded hops counted from the right. Use `0` for direct access. |

Only API requests from configured middleware peers may supply a single valid `X-Forwarded-For` address. Missing or malformed values fall back to the peer address. Other IP headers are ignored. Public redirect and admin IP behavior stays unchanged.

Keep middleware's existing YOURLS API URL and signature settings. No YOURLS core edit, database migration or signature rotation is needed. The plugin works with both the older and current YOURLS IP filters. Avoid combining it with another API IP-rewrite plugin without checking the resulting behavior.

## Failure recovery

Plugin 1.1.0 catches runtime exceptions and PHP errors in registration and IP attribution. An attribution failure falls back to the validated connection peer, or an empty address if validation itself is unavailable. It does not reuse an untrusted forwarded value. Duplicate includes and conflicting helper names leave the existing hooks alone. Web/admin behavior stays unchanged.

Recovery logs use a fixed message and omit exception values, addresses and headers. A logging failure is also contained. The plugin does not retry link creation or change existing records when attribution fails.

Keep `restart: unless-stopped` on the YOURLS service in your Docker/Portainer stack, with its existing persistent database and plugin volumes. This restarts an exited container. A health check marking a running container unhealthy does not by itself trigger a Docker restart.

Syntax errors in the plugin file, memory exhaustion and killed processes cannot all be recovered by a PHP callback. Lint the replacement before deployment and replace files atomically, retaining the previous copy for rollback. A broken file needs correction or rollback. Restarting cannot repair its code.

```bash
php -l plugins/yourls/appbox-middleware-ip/plugin.php
```

## Tests

Run these commands from the middleware repository root with PHP 8.4 or later:

```bash
php plugins/yourls/tests/client_ip_test.php api
php plugins/yourls/tests/client_ip_test.php web
php plugins/yourls/tests/client_ip_test.php undefined
php plugins/yourls/tests/recovery_test.php duplicate
php plugins/yourls/tests/recovery_test.php registration
php plugins/yourls/tests/recovery_test.php collision
```

| Parameter | Default | Coverage |
|---|---|---|
| `api` | Yes | Trust configuration, IPv4/IPv6 normalization, untrusted peers, malformed and multiple addresses, missing headers and alternate-header isolation |
| `web` | No | Preserve the original IP result for non-API requests |
| `undefined` | No | Preserve the original IP result when the API constant is absent |

The local tests use synthetic addresses and a minimal YOURLS hook stub. They need no database, credentials or network access. CI runs all three modes and eight recovery scenarios using a disposable PHP container, including disabled configuration, validation and logging functions. The private coordinated runner additionally checks the plugin against actual released and current YOURLS source.

Reference: [YOURLS proxy documentation](https://yourls.org/docs/guide/advanced/proxy).

For the recovery scenarios that disable PHP functions, run `php -d disable_functions=getenv plugins/yourls/tests/recovery_test.php getenv`. Substitute `filter_var`, `inet_pton` or `inet_ntop` in both positions to test validation failures. The `logger` scenario uses `disable_functions=getenv,error_log`.
