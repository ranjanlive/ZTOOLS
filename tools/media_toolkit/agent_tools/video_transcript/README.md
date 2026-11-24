# Video Transcript (`video_transcript`)

Fetch the transcript of an online video from its subtitles or automatic captions, as clean text, timestamped text or SRT.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Get what is said in an online video (YouTube and other yt-dlp sites) from its subtitles or automatic captions, without downloading the video. Use it to summarise, quote, translate or search a video. Formats: text (clean paragraphs, the best for reading), timestamped ([m:ss] every ~20 seconds, best for citing moments) or srt. Rolling duplicates in automatic captions are removed. Long transcripts are saved to a file in the workspace and the beginning is returned.

## Requirements

- **yt-dlp** (recommended; YouTube and 1000+ sites): `pip install -U yt-dlp`, `winget install yt-dlp.yt-dlp` or `brew install yt-dlp`. For YouTube only, **pytubefix** also works: `pip install -U pytubefix` (needs Python 3.8+). Set `ZTOOLS_YTDLP` or `ZTOOLS_PYTHON` if they are not on `PATH`.
- No FFmpeg needed.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | Address of the video page (http or https). |
| `language` | string |  | Caption language code, such as en, hi, es or pt-BR. Default en. |
| `format` | text \| timestamped \| srt |  | text, timestamped or srt. Default text. |
| `save_to` | string |  | Also save the transcript to this file, relative to the workspace. |
| `max_chars` | integer |  | Maximum characters to return; longer transcripts are saved to a file. Default 40000. |
| `backend` | auto \| yt-dlp \| pytubefix |  | auto (default), yt-dlp or pytubefix. |
| `overwrite` | boolean |  | Replace the output file if it already exists. Default false. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "url": "https://www.youtube.com/watch?v=jNQXAC9IVRw",
  "format": "timestamped"
}
```

From a terminal, in the folder that holds the files:

```bash
video_transcript --args '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw", "format": "timestamped"}'
echo '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw", "format": "timestamped"}' | video_transcript --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install video_transcript
zpilot tools install video_transcript
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
