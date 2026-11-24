# GIF & WebP Maker (`video_to_gif`)

Turn a video clip into a high-quality animated GIF (optimised palette) or a smaller animated WebP.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Make an animated GIF (or animated WebP, which is much smaller, when the output ends in .webp) from a part of a video, using an optimised colour palette for clean results. Choose start and end/duration (up to 60 s), frame rate and width.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `start` | time |  | Start of the clip. Default the beginning. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `end` | time |  | End of the clip. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `duration` | time |  | Clip length instead of end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `fps` | number |  | Frames per second (1–50). Default 12. |
| `width` | integer |  | Width in pixels (32–1920). Default 480. |
| `loop` | boolean |  | Loop forever. Default true. |
| `output` | string |  | Path for the result, relative to the workspace. Use .gif (default) or .webp. Default: next to the input as <name>_anim.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "reaction.mp4",
  "start": "0:03",
  "duration": 4,
  "width": 400
}
```

From a terminal, in the folder that holds the files:

```bash
video_to_gif --args '{"input": "reaction.mp4", "start": "0:03", "duration": 4, "width": 400}'
echo '{"input": "reaction.mp4", "start": "0:03", "duration": 4, "width": 400}' | video_to_gif --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_to_gif
zpilot tools install video_to_gif
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
