# Text Web Gateway

## Flow
- User selects gateway -> enters URL.
- SSH gateway desk (`G`) includes six terminal-native tools:
  - `W` text web browser
  - `E` email gateway send
  - `F` RSS/Atom feed reader
  - `S` article summarizer
  - `J` JSON API explorer
  - `A` generative AI prompt client
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
- Web companion also provides `FileBase` mode at `/gateway?view=files`:
  - browse/search indexed files (query + tags + area)
  - set per-user ratings
  - manage per-user download queue
  - issue short-lived ticket links and download via `/gateway?download=<token>`
  - stream queued files as a batch ZIP via `/gateway?view=files&batch=1`
- Modern gateway hub modes:
  - `/gateway?view=browser` text web browser door
  - `/gateway?view=email` email relay diagnostics door
  - `/gateway?view=ai` generative AI client door
  - `/gateway?view=rss` RSS/Atom feed reader door
  - `/gateway?view=summarize` article summarizer door
  - `/gateway?view=json` JSON API explorer door

## Safety
- SSRF deny-by-default:
  - reject `127.0.0.1`, `::1`, RFC1918, RFC6598, RFC1918, link-local, metadata IP ranges
  - reject `file://`, `gopher://`, local unix sockets, unsupported schemes
- Optional allowlist domain mode for stricter installs
- Outbound redirect policy:
  - at most 5 hops
  - preserve denylist after redirect too
- AI gateway uses the same outbound URL safety model for remote HTTPS endpoints.
- Private or loopback AI endpoints require explicit operator opt-in with `WOLFBBS_GATEWAY_AI_ALLOW_PRIVATE=1`.

## Offline Reader
- `Save for offline reading` stores extracted text in per-user folder path:
  - `offline/<user_handle>/<timestamp>-<slug>.txt`
- Offline list is displayed in web companion and future SSH reader mode.

## File Download Tickets
- Tickets are stored server-side with expiry and one-time use semantics.
- Default ticket TTL is 15 minutes.
- Ticket access is scoped to the owning user unless a `sysop` is performing the request.

## Upload Intake Safety
- Admin upload requests are bounded by `WOLFBBS_UPLOAD_MAX_BYTES` (default `33554432` bytes / 32 MiB).
- Optional upload policy hook:
  - set `WOLFBBS_UPLOAD_POLICY_HOOK` to a command
  - WolfBBS exports file metadata as env vars (`WOLFBBS_UPLOAD_PATH`, `WOLFBBS_UPLOAD_NAME`, `WOLFBBS_UPLOAD_SHA256`, `WOLFBBS_UPLOAD_SIZE_BYTES`, `WOLFBBS_UPLOAD_AREA_NAME`, `WOLFBBS_UPLOAD_UPLOADER`, etc.)
  - non-zero exit rejects the upload and removes the staged file
