# Video Frame Extractor (`video_frames`)

Pull still frames or a labelled contact sheet out of a video so vision-capable agents can see what happens in it.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Extract still images from a video so you can look at it. Modes: count (N frames spread evenly, default 6), interval (one frame every N seconds), timestamps (frames at exact times) or contact_sheet (one grid image of N frames with time labels, the cheapest way to see a whole video). Images are attached to the result when the model supports vision, and saved in the workspace.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `mode` | count \| interval \| timestamps \| contact_sheet |  | count (default), interval, timestamps or contact_sheet. |
| `count` | integer |  | Number of frames for count and contact_sheet (1–100). Default 6, or 12 for a contact sheet. |
| `interval` | time |  | For mode interval: seconds between frames, e.g. "10". |
| `timestamps` | time[] |  | For mode timestamps: the times to capture. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `width` | integer |  | Width of each frame in pixels (height keeps the aspect ratio). Default 640, or 320 in a contact sheet. |
| `image_format` | jpg \| png |  | jpg (default) or png. |
| `columns` | integer |  | Columns in a contact sheet. Default: a square-ish grid. |
| `labels` | boolean |  | Stamp the time on each frame. Default true for contact sheets, false otherwise. |
| `output_dir` | string |  | Folder for the frames, relative to the workspace. Default <name>_frames next to the video. |
| `attach` | boolean |  | Attach the images to the result for vision models. Default true. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "clips/demo.mp4",
  "mode": "contact_sheet",
  "count": 12
}
```

From a terminal, in the folder that holds the files:

```bash
video_frames --args '{"input": "clips/demo.mp4", "mode": "contact_sheet", "count": 12}'
echo '{"input": "clips/demo.mp4", "mode": "contact_sheet", "count": 12}' | video_frames --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_frames
zpilot tools install video_frames
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
