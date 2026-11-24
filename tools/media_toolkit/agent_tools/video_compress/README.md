# Video Compressor (`video_compress`)

Shrink videos to a target size (e.g. under 25 MB for Discord or email) with two-pass encoding, or to a quality level.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Make a video file smaller. Give target_size_mb to hit a size limit (for example 25 for Discord, 16 for WhatsApp, 10 for email); it uses two-pass encoding to get as close as possible with the best quality. Otherwise quality high/medium/low picks a size/quality balance. max_height (e.g. 720) lowers the resolution for big savings; codec h265 is ~40% smaller but less widely supported.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `target_size_mb` | number |  | Target file size in megabytes. Uses two-pass encoding. |
| `quality` | high \| medium \| low |  | Without a target size: high, medium (default) or low. |
| `max_height` | integer |  | Scale down to at most this many pixels high, e.g. 720 or 480. |
| `codec` | h264 \| h265 |  | h264 (default, plays everywhere) or h265 (smaller). |
| `audio_bitrate` | string |  | Audio bitrate such as 96k or 128k. Default 128k. |
| `output` | string |  | Path for the result, relative to the workspace. mp4 (default), mkv or mov. Default: next to the input as <name>_compressed.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "recording.mp4",
  "target_size_mb": 25,
  "max_height": 720
}
```

From a terminal, in the folder that holds the files:

```bash
video_compress --args '{"input": "recording.mp4", "target_size_mb": 25, "max_height": 720}'
echo '{"input": "recording.mp4", "target_size_mb": 25, "max_height": 720}' | video_compress --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_compress
zpilot tools install video_compress
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
