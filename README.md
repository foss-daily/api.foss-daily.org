# api.foss-daily.org

![Gitea Last Commit](https://img.shields.io/gitea/last-commit/foss-daily/api.foss-daily.org?display_timestamp=committer&gitea_url=https%3A%2F%2Fgitea.foss-daily.org&style=plastic)

Free, self-hostable multi-purpose REST API. No auth, no tracking, no bullshit.

Built in Go.

## Docs

Interactive API docs available at [api.foss-daily.org/docs](https://api.foss-daily.org/docs).

## Endpoints

### `GET /v1/me`
Returns your IP address as plaintext.

### `GET /v1/geo/{ip}`
Returns geolocation data for an IP. Use `me` instead of an IP to geolocate yourself.

```json
{
  "ip": "8.8.8.8",
  "country": "United States",
  "country_code": "US",
  "continent": "North America",
  "continent_code": "NA",
  "loc": "37.7510,-97.8220",
  "timezone": "America/Chicago",
  "asn": "AS15169",
  "as_name": "Google LLC"
}
```

### `GET /v1/geo/{ip}/{field}`
Returns a single field as plaintext.

Available fields: `city`, `region`, `country`, `country_code`, `continent`, `continent_code`, `loc`, `postal`, `timezone`, `asn`, `as_name`

```
GET /v1/geo/8.8.8.8/country → United States
GET /v1/geo/me/timezone     → America/Chicago
```

### `POST /v1/geo/batch`
Bulk lookup, up to 100 IPs per request.

```json
["8.8.8.8", "1.1.1.1"]
```

### `GET /v1/uuid`
Returns a random UUID v4.

### `GET /v1/echo`
Returns your request headers as plaintext. Useful for debugging proxies.

### `GET /v1/bandwidth`
Returns server bandwidth stats (production only/foss-daily.org).

### `GET /v1/usage`
Returns vnstati image (production only/foss-daily.org).

### `GET /healthz`
Returns 200 OK. For uptime monitoring.

## Rate Limits

| Endpoint | Rate |
|---|---|
| `/v1/geo/*` | 10 req/s, burst 20 |
| `/v1/geo/batch` | 2 req/s, burst 5 |
| `/v1/uuid`, `/v1/me`, `/v1/echo` | 50 req/s, burst 100 |

Exceeding limits returns `429 Too Many Requests`.

## Self-Hosting

### Requirements
- Go 1.25+
- FreeBSD or Linux
- MaxMind GeoLite2 databases (free, requires account at maxmind.com)

### Build
```sh
git clone https://gitea.foss-daily.org/foss-daily/api.foss-daily.org
cd api.foss-daily.org
make or gmake if you're on FreeBSD
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `FOSS_DAILY_CITY_DB` | `/usr/local/share/GeoLite2/GeoLite2-City.mmdb` | Path to GeoLite2-City database |
| `FOSS_DAILY_ASN_DB` | `/usr/local/share/GeoLite2/GeoLite2-ASN.mmdb` | Path to GeoLite2-ASN database |
| `FOSS_DAILY_IP_HEADER` | `X-Forwarded-For` | Header to read real IP from |
| `FOSS_DAILY_IFACE` | `em0` | Network interface for bandwidth stats |
| `FOSS_DAILY_RESOLVE_HOSTNAME` | `0` | Set to `1` to enable reverse DNS (adds latency) |
| `FOSS_DAILY_PROD` | `0` | Set to `1` to enable production-only foss-daily.org endpoints (fully optional) |

### Run
```sh
FOSS_DAILY_CITY_DB=/path/to/GeoLite2-City.mmdb \
FOSS_DAILY_ASN_DB=/path/to/GeoLite2-ASN.mmdb \
./api
```

Listens on `:6969` by default.

## Geo Data

Powered by [MaxMind GeoLite2](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data).

## Compatibility

Tested on FreeBSD and Linux. Other BSDs or systems may work but are untested.
