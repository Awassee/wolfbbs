# Text Web Gateway

## Flow
- User selects gateway -> enters URL.
- Server fetches with strict defaults:
  - timeout: 10s
  - max body: 2 MiB
  - allowed content types: `text/html`, `text/plain`
- Extract readable text:
  - strip `script`, `style`, comments
  - preserve headings/paragraphs/lists/links order
  - convert to wrapped lines at 78 columns
- Present in ANSI pager with `-- More --`.
- In the web companion, results are HTML-escaped into a `<pre>` view and the same limits apply.
- Optional "save for offline reading" writes:
  - `WOLFBBS_OFFLINE_DIR/<handle>/<timestamp>-<slug>.txt`

## Safety
- SSRF deny-by-default:
  - reject `127.0.0.1`, `::1`, RFC1918, RFC6598, RFC1918, link-local, metadata IP ranges
  - reject `file://`, `gopher://`, local unix sockets, unsupported schemes
- Optional allowlist domain mode for stricter installs
- Outbound redirect policy:
  - at most 5 hops
  - preserve denylist after redirect too

## Offline Reader
- `Save for offline reading` stores extracted text in per-user folder path:
  - `offline/<user_handle>/<timestamp>-<slug>.txt`
- Offline list is displayed in web companion and future SSH reader mode.
