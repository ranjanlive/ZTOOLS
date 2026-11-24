# Media Trimmer (`media_trim`)

Cut a clip out of a video or audio file between two timestamps, frame-accurately or instantly without re-encoding.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Cut a section out of a video or audio file. Give start and end (or start and duration); times accept seconds or clock time. mode accurate (default) re-encodes for frame-exact cuts; mode fast copies the streams instantly but cuts at the nearest keyframe. The output keeps the input's format unless the output name has another extension.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video or audio file, relative to the workspace. |
| `start` | time |  | Where the clip begins. Default the beginning. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `end` | time |  | Where the clip ends. Default the end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `duration` | time |  | Clip length, instead of end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_trim.<ext>. An existing file is never replaced unless overwrite is true. |
| `mode` | accurate \| fast |  | accurate (default, frame-exact, re-encodes) or fast (instant, no re-encoding, cuts at keyframes). |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "talk.mp4",
  "start": "1:30",
  "end": "2:45"
}
```

From a terminal, in the folder that holds the files:

```bash
media_trim --args '{"input": "talk.mp4", "start": "1:30", "end": "2:45"}'
echo '{"input": "talk.mp4", "start": "1:30", "end": "2:45"}' | media_trim --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_trim
zpilot tools install media_trim
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
