// Resumable downloads of the model files into the browser cache.
//
// Transformers.js reads a model file from Cache Storage when it finds one
// under the file's URL, so the worker fills that cache before loading the
// model. A file is downloaded in parts with HTTP range requests, and each part
// is saved as it arrives. If the page is closed or the connection drops, the
// next attempt skips the parts already saved. Once every part is in, they are
// streamed into the one cache entry Transformers.js reads, and deleted.
//
// Nothing here holds a whole file in memory: on a phone the browser closes a
// page that uses too much, and the model's weights are 1.4 GB.

export const PART_SIZE = 64 * 1024 * 1024;
export const PARTS_CACHE = "help-agent-download-parts";

/** Thrown when the server answers a range request with the whole file. */
class RangeUnsupportedError extends Error {}

function partKey(url, index, partSize) {
  return `${url}${url.includes("?") ? "&" : "?"}part=${index}&part-size=${partSize}`;
}

/** Reads a stream into a cache entry, reporting each chunk's size. */
function counted(body, onBytes) {
  return body.pipeThrough(
    new TransformStream({
      transform(chunk, controller) {
        onBytes(chunk.byteLength);
        controller.enqueue(chunk);
      },
    }),
  );
}

async function put(cache, key, response) {
  try {
    await cache.put(key, response);
  } catch (error) {
    if (error?.name === "QuotaExceededError") {
      throw new Error("The browser has no room to store the model. It needs about 3 GB of free storage while it downloads.");
    }
    throw error;
  }
}

/**
 * Downloads urls into cache, skipping files already there.
 *
 * @param {Cache} cache the cache Transformers.js reads
 * @param {string[]} urls
 * @param {(progress: {phase: "download" | "save", loaded: number, total: number}) => void} onProgress
 * @param {{partSize?: number, partsCache?: Cache}} [options]
 */
export async function downloadToCache(cache, urls, onProgress, options = {}) {
  const partSize = options.partSize ?? PART_SIZE;
  const parts = options.partsCache ?? (await caches.open(PARTS_CACHE));

  // Progress is summed over every file. A file's size is known once its
  // first part has been requested.
  const files = new Map(urls.map((url) => [url, { loaded: 0, total: 0 }]));
  const report = (phase) => {
    let loaded = 0;
    let total = 0;
    for (const f of files.values()) {
      loaded += f.loaded;
      total += f.total;
    }
    onProgress({ phase, loaded, total });
  };

  for (const url of urls) {
    const file = files.get(url);
    const cached = await cache.match(url);
    if (cached) {
      file.total = file.loaded = Number(cached.headers.get("content-length")) || 0;
      await cached.body?.cancel();
      continue;
    }
    try {
      await downloadInParts(cache, parts, url, partSize, file, report);
    } catch (error) {
      if (!(error instanceof RangeUnsupportedError)) throw error;
      // The server sent the whole file instead of a part. Stream it in one go.
      file.loaded = 0;
      await downloadWhole(cache, url, file, report);
    }
  }
}

async function downloadWhole(cache, url, file, report) {
  const response = await fetch(url);
  if (!response.ok || !response.body) {
    throw new Error(`Downloading ${url.split("/").pop()} failed: HTTP ${response.status}`);
  }
  file.total = Number(response.headers.get("content-length")) || 0;
  const body = counted(response.body, (n) => {
    file.loaded += n;
    report("download");
  });
  await put(cache, url, new Response(body, { headers: response.headers }));
}

/** Fetches bytes start..end of url, checking the server sent that range. */
async function fetchRange(url, start, end) {
  const response = await fetch(url, { headers: { Range: `bytes=${start}-${end}` } });
  if (response.status === 200) {
    await response.body?.cancel();
    throw new RangeUnsupportedError();
  }
  if (response.status !== 206 || !response.body) {
    throw new Error(`Downloading ${url.split("/").pop()} failed: HTTP ${response.status}`);
  }
  const m = /bytes (\d+)-(\d+)\/(\d+)/.exec(response.headers.get("content-range") ?? "");
  if (!m || Number(m[1]) !== start) {
    await response.body.cancel();
    throw new RangeUnsupportedError();
  }
  return { response, total: Number(m[3]), length: Number(m[2]) - start + 1 };
}

async function downloadInParts(cache, parts, url, partSize, file, report) {
  // The first part tells the file's size. A saved part 0 records it.
  let total = 0;
  const first = await parts.match(partKey(url, 0, partSize));
  if (first) {
    total = Number(first.headers.get("x-total-size")) || 0;
    await first.body?.cancel();
  }

  const savePart = async (index, fetched) => {
    const body = counted(fetched.response.body, (n) => {
      file.loaded += n;
      report("download");
    });
    const headers = {
      "content-type": "application/octet-stream",
      "content-length": String(fetched.length),
      "x-total-size": String(fetched.total),
    };
    await put(parts, partKey(url, index, partSize), new Response(body, { headers }));
  };

  let fetchedFirst = false;
  if (!total) {
    fetchedFirst = true;
    const fetched = await fetchRange(url, 0, partSize - 1);
    total = fetched.total;
    file.total = total;
    await savePart(0, fetched);
  }
  file.total = total;
  const count = Math.max(1, Math.ceil(total / partSize));

  // Count parts saved by an earlier attempt, then fetch the rest. A part
  // whose download was cut off was never stored, so it is fetched again.
  for (let index = fetchedFirst ? 1 : 0; index < count; index++) {
    const start = index * partSize;
    const length = Math.min(partSize, total - start);
    const saved = await parts.match(partKey(url, index, partSize));
    if (saved && Number(saved.headers.get("content-length")) === length) {
      await saved.body?.cancel();
      file.loaded += length;
      report("download");
      continue;
    }
    await savePart(index, await fetchRange(url, start, start + length - 1));
  }

  // Join the parts into the entry Transformers.js reads, one part at a time.
  let next = 0;
  let joined = 0;
  const joinedBody = new ReadableStream({
    async pull(controller) {
      if (next >= count) {
        controller.close();
        return;
      }
      const part = await parts.match(partKey(url, next++, partSize));
      if (!part?.body) {
        controller.error(new Error("A downloaded part of the model went missing. Try again."));
        return;
      }
      const reader = part.body.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        controller.enqueue(value);
        joined += value.byteLength;
        file.loaded = joined;
        report("save");
      }
    },
  });
  file.loaded = 0;
  await put(
    cache,
    url,
    new Response(joinedBody, {
      headers: { "content-type": "application/octet-stream", "content-length": String(total) },
    }),
  );
  file.loaded = total;

  for (let index = 0; index < count; index++) {
    await parts.delete(partKey(url, index, partSize));
  }
}
