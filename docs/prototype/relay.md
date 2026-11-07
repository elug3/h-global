# Broadcast relay prototype

> Status: prototype, 2026-11-07. Built in hg-server (`internal/relay`). Answers plan section 5.5's
> question: can frames reach hg-server faster than YouTube shows them?

## Result

Measured in hg-server's test suite on one machine (publisher, relay and a local stand-in for
YouTube all on localhost), with SRT's packet-loss buffer at 300 ms:

| Path | Delay after the picture changed |
|---|---|
| Publisher → relay → **frame in hg-server's buffer** | **≈ 0.56 s** (upper bound: includes the test publisher's own start-up) |
| Publisher → relay → copy → **YouTube stand-in** | ≈ 1.6 s (≈ 1.05 s after the frame tap) |
| Real YouTube | the stand-in's delay **plus** YouTube's own 5–30 s |

So on a good network a frame is in H-Global Studio in about half a second, while viewers see it on
YouTube 5–30 seconds later. That is the gap the plan relies on for quick listing and hg-agent.

**Not measured yet:** a real camera over a real uplink to the real server, and real YouTube. Use the
procedure below on H-Global's server.

## How it works

```
OBS / camera ── SRT (passphrase = show's ingest key) ──▶ ffmpeg #1 (ingest)
                                                          ├─ copy, unchanged ──▶ udp://127.0.0.1:19400 ──▶ ffmpeg #2 (push) ──▶ YouTube RTMPS
                                                          └─ 2 fps JPEG ──▶ hg-server ring buffer (120 s)
                                                                                  ├─ GET  /studio-api/shows/{id}/frames
                                                                                  ├─ GET  /studio-api/shows/{id}/preview   (MJPEG)
                                                                                  └─ POST /studio-api/shows/{id}/captures/relay → quick listing
```

- **Only while a show is live.** hg-server checks every 2 seconds which show is live, starts the
  ingest listener for it and stops it when the show ends. One show is live at a time.
- **Ingest key per show.** The SRT passphrase is derived from `HG_RELAY_SECRET`, the show and a
  version; nothing secret is stored. `POST …/ingest/rotate` replaces it. A wrong passphrase is
  refused (tested).
- **Two ffmpeg processes.** Ingest writes an unchanged copy to a local UDP port; a separate pusher
  sends it to YouTube. If YouTube drops, only the pusher restarts and frames keep coming. (ffmpeg's
  `tee` output was tried first; it cannot write the FLV header for H.264 copied from MPEG-TS.)
- **Replay offsets.** The first frame sets the show's `ingest_started_at`. A capture's
  `broadcast_offset_sec` is its frame's arrival time minus that, which is where it sits on YouTube's
  timeline.
- **No re-encoding to YouTube.** The copy is the original stream, so quality is the camera's and
  the CPU cost is only the 2 fps JPEG decode.

## Differences from the plan

| Plan (5.5) | Prototype | Why |
|---|---|---|
| A media server (e.g. MediaMTX) beside hg-server | ffmpeg only, started by hg-server | Running a downloaded MediaMTX binary was not allowed in this environment; ffmpeg from the OS packages does everything the prototype needs for one live show. MediaMTX can replace ffmpeg #1 later. |
| WebRTC preview | MJPEG preview at 2 fps (`<img src=…/preview>`) | Needs no extra server. WebRTC (smooth video) comes with MediaMTX. |
| SRT or RTMP ingest | SRT only | SRT has a passphrase and recovers lost packets; RTMP would put the key in the URL. |
| Audio tap for speech-to-text | Not built | Phase 2, with hg-agent. |

If ffmpeg #1 crashes, the broadcast to YouTube stops until it restarts (about 1 second, and the
broadcaster's OBS reconnects). Keep OBS's direct-to-YouTube profile as the fallback (plan D9).

## Try it

```bash
# hg-server with the relay (see hg-server/README.md for the other settings)
export HG_RELAY_ENABLED=true HG_RELAY_SECRET='a-long-random-secret' HG_RELAY_PUBLIC_HOST='your.server:9000'
export HG_YOUTUBE_STREAM_KEY='xxxx-xxxx-xxxx-xxxx'   # optional: also push to YouTube
go run ./cmd/hg-server

# In H-Global Studio (or with curl): create a show, then
GET  /studio-api/shows/{id}/ingest    → {"url": "srt://your.server:9000?passphrase=…&latency=300000", …}
POST /studio-api/shows/{id}/start     → the relay starts listening within 2 s
```

OBS: Settings → Stream → Service **Custom**, Server = the `url` above, Stream key empty.
Output: x264 or hardware H.264, keyframe interval 1–2 s, 4–6 Mbps for 720p. Open UDP port 9000.

A test broadcast without a camera:

```bash
ffmpeg -re -f lavfi -i testsrc2=size=1280x720:rate=30 -f lavfi -i sine=frequency=440:sample_rate=48000 \
  -c:v libx264 -preset veryfast -tune zerolatency -g 30 -pix_fmt yuv420p -c:a aac -f mpegts \
  "srt://your.server:9000?passphrase=PASSPHRASE&latency=300000"
```

## Measuring on H-Global's server

1. Point a camera at a phone showing a stopwatch with milliseconds, and broadcast with OBS from
   the place you will stream from (or a similar network).
2. Open `/studio-api/shows/{id}/preview` and the YouTube watch page side by side on one screen,
   next to the stopwatch.
3. Take a photo of the screen. Relay delay = stopwatch − preview; YouTube delay = stopwatch −
   YouTube. Repeat with YouTube's latency setting on "Ultra low" and "Normal".
4. Check `GET /studio-api/shows/{id}/relay` during the test: `receiving` should stay true,
   `push_restarts` 0.
5. Write the numbers into this document.

What to check on the server: upload bandwidth (the stream goes out again to YouTube, about
6 Mbps), UDP 9000 open, and CPU while decoding (one core is plenty for 2 fps).

## Next

- Run the measurement above on the real server and uplink.
- If smooth preview matters, add MediaMTX (or another media server) for ingest and WebRTC preview;
  hg-server keeps the same buffer and routes.
- Audio tap for speech-to-text when hg-agent starts (plan 5.6).
