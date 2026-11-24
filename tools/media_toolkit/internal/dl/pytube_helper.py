# SPDX-License-Identifier: Apache-2.0
# Helper for the ZTools media toolkit: YouTube access through pytubefix.
# Usage: python ztools_pytube_helper.py request.json
# Prints "PROGRESS <n>%" lines while downloading and one JSON object as the
# last line of stdout: the result, or {"error": "..."}.
import json
import os
import re
import sys


def emit(obj):
    sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n")
    sys.stdout.flush()


try:
    from pytubefix import Playlist, YouTube
except Exception as e:  # pragma: no cover
    emit({"error": "pytubefix is not available: %s" % e})
    sys.exit(0)

CLIENTS = (None, "WEB", "ANDROID_VR", "MWEB", "TV")


def progress(stream, chunk, remaining):
    total = getattr(stream, "filesize", 0) or 0
    if total:
        sys.stdout.write("PROGRESS %.1f%%\n" % (100.0 * (total - remaining) / total))
        sys.stdout.flush()


def open_video(url, need_streams=True, with_progress=False):
    last = None
    for client in CLIENTS:
        try:
            kw = {}
            if client:
                kw["client"] = client
            if with_progress:
                kw["on_progress_callback"] = progress
            yt = YouTube(url, **kw)
            _ = yt.title
            if need_streams:
                _ = yt.streams.first()
            return yt
        except Exception as e:  # try the next client
            last = e
    raise RuntimeError("YouTube refused the request (%s). Installing yt-dlp usually works better." % last)


def safe_name(title, vid):
    name = re.sub(r'[\\/:*?"<>|\x00-\x1f]', "_", title or "video").strip(" .")[:150]
    return "%s [%s]" % (name or "video", vid)


def height(stream):
    try:
        return int((stream.resolution or "0p").rstrip("p"))
    except ValueError:
        return 0


def is_playlist(url):
    return "list=" in url and "watch?v=" not in url and "youtu.be/" not in url


def video_urls(url, limit):
    if is_playlist(url):
        if limit <= 0:
            raise RuntimeError("this is a playlist URL; set playlist_limit to download its videos")
        return list(Playlist(url).video_urls)[:limit]
    return [url]


def pick_caption(yt, lang):
    caps = list(yt.captions)
    codes = [c.code for c in caps]
    for want in (lang, "a." + lang):
        for c in caps:
            if c.code == want:
                return c
    for c in caps:
        if c.code.startswith(lang) or c.code.startswith("a." + lang):
            return c
    raise RuntimeError("no %r captions; available: %s" % (lang, ", ".join(codes) or "none"))


def download(req):
    out = req["out"]
    max_h = int(req.get("max_height") or 0)
    limit_mb = int(req.get("max_filesize_mb") or 0)
    items = []
    for url in video_urls(req["url"], int(req.get("playlist_limit") or 0)):
        yt = open_video(url, True, True)
        base = safe_name(yt.title, yt.video_id)
        item = {"id": yt.video_id, "title": yt.title, "author": yt.author, "length": yt.length}
        streams = yt.streams
        chosen = []
        if req.get("kind") == "audio":
            a = streams.get_audio_only()
            if a is None:
                raise RuntimeError("no audio stream is available")
            chosen.append(("audio", a, base + ".m4a"))
        else:
            vids = [s for s in streams.filter(adaptive=True, only_video=True, file_extension="mp4").order_by("resolution").desc()
                    if not max_h or height(s) <= max_h]
            prog = [s for s in streams.filter(progressive=True, file_extension="mp4").order_by("resolution").desc()
                    if not max_h or height(s) <= max_h]
            if vids and (not prog or height(vids[0]) > height(prog[0])):
                a = streams.filter(only_audio=True, file_extension="mp4").order_by("abr").desc().first()
                chosen.append(("video", vids[0], "v_%s.mp4" % yt.video_id))
                if a is not None:
                    chosen.append(("audio", a, "a_%s.m4a" % yt.video_id))
            elif prog:
                chosen.append(("single", prog[0], base + ".mp4"))
            else:
                raise RuntimeError("no downloadable video stream matches max_height=%d" % max_h)
        if limit_mb:
            size = sum((s.filesize or 0) for _, s, _ in chosen)
            if size > limit_mb * 1024 * 1024:
                raise RuntimeError("the download would be %.0f MB, larger than max_filesize_mb (%d MB)" % (size / 1048576.0, limit_mb))
        for role, s, name in chosen:
            path = s.download(output_path=out, filename=name, skip_existing=False)
            if role == "audio" and req.get("kind") == "audio":
                item["audio"] = path
            else:
                item[role] = path
        if req.get("subtitles"):
            try:
                cap = pick_caption(yt, req.get("sub_lang") or "en")
                p = os.path.join(out, "%s.%s.srt" % (base, cap.code.replace("a.", "")))
                with open(p, "w", encoding="utf-8") as f:
                    f.write(cap.generate_srt_captions())
                item["subtitles"] = p
            except Exception:
                pass
        item["base"] = base
        items.append(item)
    return {"items": items}


def info(req):
    url = req["url"]
    if is_playlist(url):
        pl = Playlist(url)
        limit = max(int(req.get("playlist_limit") or 0), 10)
        urls = list(pl.video_urls)
        entries = []
        for v in pl.videos[:limit]:
            try:
                entries.append({"title": v.title, "url": v.watch_url, "length": v.length})
            except Exception:
                entries.append({"title": "", "url": v.watch_url, "length": 0})
        return {"kind": "playlist", "id": pl.playlist_id, "title": pl.title, "author": getattr(pl, "owner", ""),
                "count": len(urls), "entries": entries}
    yt = open_video(url, True)
    heights = sorted({height(s) for s in yt.streams.filter(type="video")} - {0})
    chapters = []
    try:
        chapters = [{"title": c.title, "start": c.start_seconds} for c in (yt.chapters or [])]
    except Exception:
        pass
    caps = []
    try:
        caps = [c.code for c in yt.captions]
    except Exception:
        pass
    pd = yt.publish_date
    return {"kind": "video", "id": yt.video_id, "title": yt.title, "author": yt.author, "length": yt.length,
            "views": yt.views, "publish_date": pd.strftime("%Y%m%d") if pd else "", "description": yt.description or "",
            "keywords": yt.keywords or [], "heights": heights, "captions": caps, "chapters": chapters}


def captions(req):
    yt = open_video(req["url"], False)
    cap = pick_caption(yt, req.get("lang") or "en")
    return {"lang": cap.code, "text": cap.generate_srt_captions(), "title": yt.title, "id": yt.video_id}


def main():
    with open(sys.argv[1], encoding="utf-8") as f:
        req = json.load(f)
    ops = {"download": download, "info": info, "captions": captions}
    try:
        emit(ops[req["op"]](req))
    except Exception as e:
        emit({"error": str(e) or e.__class__.__name__})


if __name__ == "__main__":
    main()
