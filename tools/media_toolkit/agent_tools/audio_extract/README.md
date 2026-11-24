# Audio Extractor (`audio_extract`)

Extract the soundtrack of a video as MP3, M4A, WAV, FLAC or Opus, optionally just a section.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Save the audio of a video (or convert an audio file) as mp3 (default), m4a, wav, flac, opus or ogg, optionally only between start and end. Use wav for speech-to-text tools, mp3 for sharing.

## Requirements

- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `input` | string | yes | Path of the audio or video file, relative to the workspace. |
| `format` | mp3 \| m4a \| wav \| flac \| opus \| ogg |  | mp3 (default), m4a, wav, flac, opus or ogg. The output extension wins if given. |
| `bitrate` | string |  | Bitrate for lossy formats, such as 128k, 192k (default) or 320k. |
| `start` | time |  | Start of the section. Default the beginning. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `end` | time |  | End of the section. Default the end. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `output` | string |  | Path for the result, relative to the workspace. The extension picks the format. Default: next to the input as <name>_audio.<ext>. An existing file is never replaced unless overwrite is true. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "input": "interview.mp4",
  "format": "mp3",
  "bitrate": "192k"
}
```

From a terminal, in the folder that holds the files:

```bash
audio_extract --args '{"input": "interview.mp4", "format": "mp3", "bitrate": "192k"}'
echo '{"input": "interview.mp4", "format": "mp3", "bitrate": "192k"}' | audio_extract --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install audio_extract
zpilot tools install audio_extract
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
