# Summary

The chat rendered every attachment as a name, a size, and a download button, so
an image the user sent or the agent returned was visible only after downloading
it. Images now appear as thumbnails in the thread and in the composer, and a
thumbnail opens the full image in a dialog.

# Design

`FileDownloadChip` is the single render point for both directions of the
conversation: user attachments resolve through it, and agent-produced files
reach it as a data content part. Branching there on an `image/` content type
covers sending and receiving in one place.

The bytes come through the files API rather than a direct `<img src>`. That
endpoint is credentialed and cross-origin, so a bare element reference sends no
session and 401s; `useDeploymentFilePreview` fetches the blob and wraps it in an
object URL, revoked when the blob changes or the component unmounts. The query
is keyed per file and never goes stale, so scrolling a thread refetches nothing.

Thread thumbnails are bounded rather than square-cropped, so a wide screenshot
and a tall portrait keep their aspect and still occupy a comparable footprint.
Expanding opens a dialog holding the image at up to 70% of viewport height, with
the download action moved there.

The composer draws its thumbnail from the staged `File` instead, since nothing
has been uploaded at that point. That covers the paperclip and drag-and-drop
paths equally, because both land in the same attachment adapter.

A preview costs the whole file, because there is no server-side thumbnail and
the store accepts uploads up to 100 MiB. Two bounds keep that from becoming the
client's problem: a thumbnail fetches only while it is on screen, and only for
files under 8 MiB. Anything larger stays a chip. The composer applies the same
size bound to the staged file, since the decode of a 100 MiB image costs the
same whether the bytes came over the network or off the local disk.

Retention is the harder half. Query cache lifetime is keyed to mount, and the
thread never unmounts a message, so a preview subscribed for the life of its
chip is held for the life of the conversation. Visibility therefore decides
whether the query exists at all: the chip mounts the preview component only
while the thumbnail is on screen, and `gcTime: 0` drops the blob as the last
observer goes away. TanStack cancels the in-flight fetch through the query
signal on the way out. Live bytes stay proportional to what is on screen rather
than to how far the user has scrolled.

The content key sits outside the `files` prefix. Upload and delete invalidate
`fileKeys.all` to refresh the listing, and that prefix would otherwise match
every preview, so attaching one file would re-download every image already on
screen. Deleting one therefore has to drop that file's bytes by hand, since the
listing invalidation no longer reaches them and the thumbnail would keep
rendering an image that no longer exists. That drop is a reset rather than a
removal: removing evicts the entry but leaves a mounted preview holding its
last result, and the thumbnail lives in the thread while the deletion happens
in the files panel.

Until the bytes arrive, and on failure, the chip renders as before. A slow or
broken fetch costs the preview, not access to the file. Bytes that arrive but
cannot be decoded, because the file is corrupt or mislabeled or the format is
unsupported, fall back the same way rather than leaving a broken image: both
thumbnails treat an `error` on the element as a failed preview. In the thread
that failure is remembered against the object URL, so a later refetch mints a
new URL and the image is retried.

# Migration

None.
