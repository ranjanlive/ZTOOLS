# ZTools Media Toolkit

**23 video, audio and online-media tools for AI agents**, powered by FFmpeg, with yt-dlp or pytubefix for downloads. Each tool works with [ZAgent / ZEngine](https://github.com/ranjanlive/zagent) and [ZPilot](https://github.com/ranjanlive/zpilot), and also runs on its own from a terminal.

An agent equipped with these tools can, for example:

- download a talk from YouTube, cut the best two minutes, reframe them to 9:16 with a blurred background, burn in captions and add music for a Short;
- read a video's transcript and chapters, then summarise or quote it;
- look at a video through a contact sheet of frames, find its scene cuts, and pull thumbnails;
- clean up a podcast: normalise loudness to −16 LUFS, reduce noise, remove long pauses and render an audiogram;
- shrink a recording to under 25 MB for Discord or email.

## Tools

| Tool | What it does |
|---|---|
| **Online media** | |
| [`media_download`](agent_tools/media_download) | Download video or audio from YouTube and 1000+ sites: quality, sections, subtitles, playlists. |
| [`media_lookup`](agent_tools/media_lookup) | Title, channel, duration, views, chapters, qualities and caption languages, without downloading. |
| [`video_transcript`](agent_tools/video_transcript) | Transcript from subtitles or automatic captions, as clean text, timestamped text or SRT. |
| **Inspect** | |
| [`media_probe`](agent_tools/media_probe) | Duration, container, streams, codecs, resolution, frame rate, bitrate, rotation, chapters. |
| [`video_frames`](agent_tools/video_frames) | Frames at intervals, counts or exact times, or a labelled contact sheet; attached for vision models. |
| [`scene_detect`](agent_tools/scene_detect) | Scene cuts with start/end times and optional thumbnails. |
| **Edit** | |
| [`media_trim`](agent_tools/media_trim) | Cut a clip, frame-accurately or instantly. |
| [`media_split`](agent_tools/media_split) | Split by segment length, into N parts, or at timestamps. |
| [`media_merge`](agent_tools/media_merge) | Join clips; mixed sizes, frame rates and silent clips are normalised automatically. |
| [`video_resize`](agent_tools/video_resize) | Resize or reframe (16:9 → 9:16, 1:1, 4:5) with crop, pad or blurred background. |
| [`video_rotate`](agent_tools/video_rotate) | Rotate 90/180/270° and mirror. |
| [`video_speed`](agent_tools/video_speed) | Speed up, slow down or reverse, keeping voice pitch natural. |
| [`video_watermark`](agent_tools/video_watermark) | Logo and/or text overlay with position, opacity and time range. |
| [`video_subtitles`](agent_tools/video_subtitles) | Burn in or attach SRT/VTT/ASS subtitles. |
| **Convert** | |
| [`media_convert`](agent_tools/media_convert) | Any format or codec: MP4, WebM, MKV, MOV, MP3, WAV, FLAC, H.265, AV1. |
| [`video_compress`](agent_tools/video_compress) | Hit a target file size with two-pass encoding, or a quality level. |
| [`video_to_gif`](agent_tools/video_to_gif) | High-quality animated GIF or WebP. |
| [`images_to_video`](agent_tools/images_to_video) | Slideshow with transitions, any resolution and background music. |
| **Audio** | |
| [`audio_extract`](agent_tools/audio_extract) | Soundtrack to MP3, M4A, WAV, FLAC or Opus. |
| [`audio_adjust`](agent_tools/audio_adjust) | Two-pass loudness normalisation, volume, fades, noise reduction, rumble filter. |
| [`audio_mix`](agent_tools/audio_mix) | Background music or voice-over with ducking, or replace the soundtrack. |
| [`silence_cut`](agent_tools/silence_cut) | Find pauses or remove them for jump-cut edits. |
| [`audio_waveform`](agent_tools/audio_waveform) | Waveform or spectrogram image, or an audiogram video. |

## Requirements

- **FFmpeg** with FFprobe, version 5 or newer recommended. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`. If it is not on `PATH`, set `ZTOOLS_FFMPEG` (and optionally `ZTOOLS_FFPROBE`) to its full path.
- **For the online tools:** [yt-dlp](https://github.com/yt-dlp/yt-dlp) (recommended, 1000+ sites): `pip install -U yt-dlp`, `winget install yt-dlp.yt-dlp` or `brew install yt-dlp`. For YouTube only, [pytubefix](https://github.com/JuanBindez/pytubefix) also works: `pip install -U pytubefix`. Override their locations with `ZTOOLS_YTDLP` and `ZTOOLS_PYTHON`.
- **For text watermarks:** a TrueType font is found automatically (Arial, Segoe UI, DejaVu, …), or set `ZTOOLS_FONT`.

None of these programs are bundled; they keep their own licenses.

## Install

```bash
zagent tools install media_trim video_frames media_download     # ZAgent / ZEngine
zpilot tools install media_trim                                 # ZPilot (or from the Tools page)
```

Each tool is its own small download (`<tool>-<version>-<os>-<arch>.zip` on the [releases page](https://github.com/ranjanlive/ZTOOLS/releases)), verified against `SHA256SUMS.txt`.

## Use from a terminal

Every tool reads its arguments as JSON and prints one JSON result:

```bash
media_trim --args '{"input": "talk.mp4", "start": "1:30", "end": "2:45"}'
echo '{"input": "talk.mp4", "mode": "contact_sheet"}' | video_frames --agent-json
ztmedia --list                       # the development binary runs any tool: ztmedia <tool> --args '{...}'
```

Times accept seconds (`90`, `12.5`) or clock time (`"1:30"`, `"01:02:03.5"`).

## Design

- **Safe by default.** Every input and output must be inside the agent's workspace (symlinks included), downloads accept only public `http(s)` URLs, and existing files are never overwritten unless `overwrite` is true.
- **Agent-friendly results.** Each result says what was produced with its duration, resolution, codecs and size; errors explain how to fix the call; misspelled parameters are reported. Long jobs report progress to the running agent.
- **One binary, many tools.** All tools share one Go program (`main.go`) that picks the tool from its own file name, so each release archive holds just that tool under its own name.
- **Tested against real FFmpeg.** The test suite generates media and runs every tool end to end, and checks every manifest against the code so the schema an agent sees always matches what the tool accepts.

## Development

```bash
cd tools/media_toolkit
go test ./...                                  # needs FFmpeg; FFmpeg tests are skipped without it
go run . --list
go run ./cmd/package -version 1.0.0            # release archives in dist/1.0.0
```

Releases are built by the [`release-media-toolkit`](../../.github/workflows/release-media-toolkit.yml) workflow when a `media-toolkit-v*` tag is pushed.

## License

Apache-2.0. Only download content you own or have permission to use, and respect each site's terms.
