# Speed & Reverse (`video_speed`)

Speed up, slow down or reverse video and audio, keeping voices at their natural pitch.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Change the playback speed of a video or audio file (0.25× to 8×), for example 2 for a quick time-lapse or 0.5 for slow motion, and/or play it backwards. Voices keep their natural pitch unless keep_pitch is false (chipmunk/deep effect). Reverse is limited to 10-minute files.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video or audio file, relative to the workspace. |
| `speed` | number |  | Playback speed multiplier from 0.25 to 8 (2 = twice as fast, 0.5 = half speed). Default 1. |
| `keep_pitch` | boolean |  | Keep voices at natural pitch when changing speed. Default true. |
| `reverse` | boolean |  | Play backwards. Default false. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_<speed>x.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "drive.mp4",
  "speed": 4
}
```

From a terminal, in the folder that holds the files:

```bash
video_speed --args '{"input": "drive.mp4", "speed": 4}'
echo '{"input": "drive.mp4", "speed": 4}' | video_speed --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_speed
zpilot tools install video_speed
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
