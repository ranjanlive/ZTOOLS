# Scene Detector (`scene_detect`)

Detect cuts and scene changes in a video and list each scene's start, end and length, with optional thumbnails.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Find the cuts and scene changes in a video. Returns a numbered list of scenes with start, end and length, which is useful for chaptering, splitting a video into shots, or finding where something changes. threshold controls sensitivity (lower finds subtler changes). With thumbnails true, one image per scene is saved and attached.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `threshold` | number |  | Scene-change sensitivity from 0.05 to 0.95. Default 0.3; use 0.15–0.2 for subtle changes, 0.4+ for hard cuts only. |
| `min_scene_length` | time |  | Ignore scenes shorter than this. Default 1 second. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `max_scenes` | integer |  | Stop after this many scenes. Default 200. |
| `thumbnails` | boolean |  | Save (and attach) one thumbnail per scene. Default false. |
| `output_dir` | string |  | Folder for thumbnails, relative to the workspace. Default <name>_scenes next to the video. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "footage/vlog.mp4",
  "thumbnails": true
}
```

From a terminal, in the folder that holds the files:

```bash
scene_detect --args '{"input": "footage/vlog.mp4", "thumbnails": true}'
echo '{"input": "footage/vlog.mp4", "thumbnails": true}' | scene_detect --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install scene_detect
zpilot tools install scene_detect
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
