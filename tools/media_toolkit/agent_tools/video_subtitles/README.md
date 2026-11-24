# Subtitle Embedder (`video_subtitles`)

Add SRT, VTT or ASS subtitles to a video, burned into the picture or as a selectable subtitle track.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Add subtitles to a video. mode burn (default) draws them into the picture so they show everywhere, including social media; mode soft adds a subtitle track that viewers can switch on and off (mp4, mov, mkv, webm) without re-encoding. Works well with transcripts from video_transcript or .srt files downloaded by media_download.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video file, relative to the workspace. |
| `subtitles` | string | yes | Subtitle file (.srt, .vtt, .ass or .ssa), relative to the workspace. |
| `mode` | burn \| soft |  | burn (default) or soft. |
| `language` | string |  | For soft subtitles: ISO 639 language code such as en or eng. |
| `font_size` | integer |  | Font size for burned SRT/VTT subtitles. Default: the renderer's default. |
| `position` | bottom \| top |  | Burned SRT/VTT subtitles at the bottom (default) or top. |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_subtitled.<ext>. An existing file is never replaced unless overwrite is true. |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "lecture.mp4",
  "subtitles": "lecture.en.srt",
  "mode": "burn"
}
```

From a terminal, in the folder that holds the files:

```bash
video_subtitles --args '{"input": "lecture.mp4", "subtitles": "lecture.en.srt", "mode": "burn"}'
echo '{"input": "lecture.mp4", "subtitles": "lecture.en.srt", "mode": "burn"}' | video_subtitles --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_subtitles
zpilot tools install video_subtitles
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
