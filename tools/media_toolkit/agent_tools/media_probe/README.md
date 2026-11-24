# Media Probe (`media_probe`)

Inspect any video or audio file: duration, container, streams, codecs, resolution, frame rate, bitrate, rotation and chapters.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Inspect a video or audio file with FFprobe: container, duration, size, overall bitrate, and every stream (codec, resolution, frame rate, pixel format, rotation, sample rate, channels, language), plus chapters. Use it before editing to learn a file's length and format, or to answer questions about a media file. format "json" returns the full FFprobe data.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video or audio file, relative to the workspace. |
| `format` | summary \| json |  | summary (readable, default) or json (complete FFprobe output). |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "clips/interview.mp4"
}
```

From a terminal, in the folder that holds the files:

```bash
media_probe --args '{"input": "clips/interview.mp4"}'
echo '{"input": "clips/interview.mp4"}' | media_probe --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_probe
zpilot tools install media_probe
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
