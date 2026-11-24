# Media Converter (`media_convert`)

Convert video and audio between formats and codecs (MP4, WebM, MKV, MOV, MP3, WAV, FLAC, H.265, AV1), with optional downscaling.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Convert a video or audio file to another format or codec. The output extension (or format) decides the container, with sensible codecs chosen automatically: mp4/mov/mkv → H.264 + AAC, webm → VP9 + Opus, mp3, m4a, wav, flac, ogg/opus. Override with video_codec (h264, h265, vp9, av1, copy) and audio_codec (aac, mp3, opus, flac, copy, none), and optionally cap the height or change the frame rate.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the video or audio file, relative to the workspace. |
| `output` | string |  | Path for the result, relative to the workspace. Its extension picks the format. Default: next to the input. |
| `format` | string |  | Target format when output is not given, such as mp4, webm, mkv, mov, mp3, m4a, wav, flac or opus. |
| `video_codec` | auto \| h264 \| h265 \| vp9 \| av1 \| copy |  | auto (default), h264, h265, vp9, av1 or copy. |
| `audio_codec` | auto \| aac \| mp3 \| opus \| flac \| copy \| none |  | auto (default), aac, mp3, opus, flac, copy or none (drop audio). |
| `quality` | high \| medium \| low |  | Encoding quality when re-encoding: high (visually lossless, larger files), medium, or low (smallest). Default high. |
| `audio_bitrate` | string |  | Audio bitrate such as 128k, 192k or 320k. Default 192k. |
| `max_height` | integer |  | Scale down to at most this many pixels high (e.g. 720). Never scales up. |
| `fps` | number |  | Change the frame rate, e.g. 30. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "screen.mov",
  "format": "mp4",
  "max_height": 1080
}
```

From a terminal, in the folder that holds the files:

```bash
media_convert --args '{"input": "screen.mov", "format": "mp4", "max_height": 1080}'
echo '{"input": "screen.mov", "format": "mp4", "max_height": 1080}' | media_convert --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_convert
zpilot tools install media_convert
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
