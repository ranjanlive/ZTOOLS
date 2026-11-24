# Video Rotate & Flip (`video_rotate`)

Rotate a video 90/180/270 degrees and/or mirror it horizontally or vertically.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Rotate a video by 90 (clockwise), 180 or 270/-90 (counter-clockwise) degrees and/or mirror it. Use it to fix sideways phone videos or mirrored webcam recordings. The picture is re-encoded, so it plays correctly in every player.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `rotate` | integer |  | Degrees clockwise: 90, 180, 270 (or -90). Default 0. |
| `flip` | none \| horizontal \| vertical \| both |  | Mirror the picture: none (default), horizontal, vertical or both. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_rotated.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "phone_clip.mp4",
  "rotate": 90
}
```

From a terminal, in the folder that holds the files:

```bash
video_rotate --args '{"input": "phone_clip.mp4", "rotate": 90}'
echo '{"input": "phone_clip.mp4", "rotate": 90}' | video_rotate --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_rotate
zpilot tools install video_rotate
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
