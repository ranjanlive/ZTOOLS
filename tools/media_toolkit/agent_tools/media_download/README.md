# Media Downloader (`media_download`)

Download videos or audio from YouTube and 1000+ sites via yt-dlp, with pytubefix as a YouTube fallback. Quality, sections, subtitles and playlists.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Download a video, or just its audio, from YouTube or any of the 1000+ sites yt-dlp supports (Vimeo, X/Twitter, Instagram, TikTok, SoundCloud, …). Use it when the user asks to save or fetch online media into the workspace. Choose kind "audio" for music, podcasts or speech (MP3 by default). You can limit the resolution, download only a section, fetch subtitles, or download the first N videos of a playlist. Files are saved in the workspace (downloads/ by default) and the result lists their paths, durations and sizes. Only download content the user owns or has permission to use.

## Requirements

- **yt-dlp** (recommended; YouTube and 1000+ sites): `pip install -U yt-dlp`, `winget install yt-dlp.yt-dlp` or `brew install yt-dlp`. For YouTube only, **pytubefix** also works: `pip install -U pytubefix` (needs Python 3.8+). Set `ZTOOLS_YTDLP` or `ZTOOLS_PYTHON` if they are not on `PATH`.
- **FFmpeg** (with FFprobe) on `PATH`, or set `ZTOOLS_FFMPEG` to its full path. Windows: `winget install Gyan.FFmpeg` · macOS: `brew install ffmpeg` · Linux: `sudo apt install ffmpeg`. FFmpeg is needed to merge high-quality video and audio streams, convert audio, and download sections.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | Address of the video, audio or playlist page (http or https). |
| `kind` | video \| audio |  | What to download: video (picture and sound) or audio (sound only). Default video. |
| `quality` | best \| 2160p \| 1440p \| 1080p \| 720p \| 480p \| 360p \| 240p |  | Highest video resolution to download. Default best. |
| `audio_format` | mp3 \| m4a \| opus \| wav \| flac |  | Format for kind "audio". Default mp3. |
| `output_dir` | string |  | Folder to save into, relative to the workspace. Default downloads. |
| `start` | time |  | Download only from this point. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `end` | time |  | Download only up to this point. Time as seconds ("90", "12.5") or clock time ("1:30", "01:02:03.5"). |
| `subtitles` | boolean |  | Also save subtitles (or automatic captions) as .srt next to the file. Default false. |
| `subtitle_language` | string |  | Subtitle language code, such as en, hi or es. Default en. |
| `playlist_limit` | integer |  | For playlist URLs: how many videos to download, from the start. 0 (default) downloads a single video only. |
| `max_filesize_mb` | integer |  | Skip the download if the file would be larger than this many megabytes. Default: no limit. |
| `backend` | auto \| yt-dlp \| pytubefix |  | Downloader to use. auto (default) uses yt-dlp when installed, otherwise pytubefix for YouTube. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "url": "https://www.youtube.com/watch?v=jNQXAC9IVRw",
  "kind": "video",
  "quality": "720p",
  "subtitles": true
}
```

From a terminal, in the folder that holds the files:

```bash
media_download --args '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw", "kind": "video", "quality": "720p", "subtitles": true}'
echo '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw", "kind": "video", "quality": "720p", "subtitles": true}' | media_download --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_download
zpilot tools install media_download
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
