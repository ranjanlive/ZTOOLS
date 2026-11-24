# Media Splitter (`media_split`)

Split a video or audio file into parts: fixed-length segments, N equal parts, or at chosen timestamps.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Split one video or audio file into several files: every N seconds (segment_length), into N equal parts (parts), or at given timestamps (at). Useful for making clips for social media, chunking long recordings for transcription, or cutting a video at chapter points. Parts are named <name>_part001, _part002, … in the output folder.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video or audio file, relative to the workspace. |
| `mode` | segment_length \| parts \| at |  | segment_length (default), parts or at. |
| `segment_length` | string |  | For mode segment_length: length of each part, e.g. "60" or "10:00". Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `parts` | integer |  | For mode parts: how many equal parts (2–500). |
| `at` | time[] |  | For mode at: increasing timestamps to split at. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output_dir` | string |  | Folder for the parts, relative to the workspace. Default <name>_parts next to the input. |
| `cut` | accurate \| fast |  | accurate (default, re-encodes, exact) or fast (no re-encoding, cuts at keyframes). |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "podcast.mp3",
  "mode": "segment_length",
  "segment_length": "10:00"
}
```

From a terminal, in the folder that holds the files:

```bash
media_split --args '{"input": "podcast.mp3", "mode": "segment_length", "segment_length": "10:00"}'
echo '{"input": "podcast.mp3", "mode": "segment_length", "segment_length": "10:00"}' | media_split --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_split
zpilot tools install media_split
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
