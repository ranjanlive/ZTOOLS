# Online Media Lookup (`media_lookup`)

Inspect an online video or playlist without downloading: title, channel, duration, views, chapters, qualities and caption languages.

Part of the [ZTools media toolkit](../../README.md).

## What it does

Look up an online video or playlist without downloading it. Returns the title, channel, duration, publish date, views, available video qualities, subtitle and automatic-caption languages, chapters, tags and description; for playlists, the list of videos with their URLs. Use it before downloading to check what is available, or to answer questions about a video.

## Requirements

- **yt-dlp** (recommended; YouTube and 1000+ sites): `pip install -U yt-dlp`, `winget install yt-dlp.yt-dlp` or `brew install yt-dlp`. For YouTube only, **pytubefix** also works: `pip install -U pytubefix` (needs Python 3.8+). Set `ZTOOLS_YTDLP` or `ZTOOLS_PYTHON` if they are not on `PATH`.
- No FFmpeg needed.

## Parameters

| Name | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | Address of the video or playlist page (http or https). |
| `playlist_limit` | integer |  | For playlists: how many entries to list. Default 25. |
| `max_description` | integer |  | Maximum characters of the description to include (0 = omit). Default 1500. |
| `backend` | auto \| yt-dlp \| pytubefix |  | auto (default), yt-dlp or pytubefix. |

## Example

From an agent (ZAgent, ZEngine or ZPilot), the model calls the tool with:

```json
{
  "url": "https://www.youtube.com/watch?v=jNQXAC9IVRw"
}
```

From a terminal, in the folder that holds the files:

```bash
media_lookup --args '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw"}'
echo '{"url": "https://www.youtube.com/watch?v=jNQXAC9IVRw"}' | media_lookup --agent-json
```

The tool prints one JSON object: `{"output": "...", "is_error": false, "media": [...]}`.

## Install

```bash
zagent tools install media_lookup
zpilot tools install media_lookup
```

## Safety

Every file the tool reads or writes must be inside the agent's workspace. Existing files are never overwritten unless `overwrite` is true.

## License

Apache-2.0. FFmpeg, yt-dlp and pytubefix are separate programs under their own licenses and are not bundled.
