import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { createHookWrapper } from "@/test/test-utils";
import { fileKeys } from "./keys";
import {
  useDeleteDeploymentFile,
  useDeploymentFilePreview,
} from "./files";

const DEPLOYMENT = "dep-1";
const KEY = "file-1";

let created: string[] = [];
let revoked: string[] = [];
let contentRequests = 0;
const realCreateObjectURL = URL.createObjectURL;
const realRevokeObjectURL = URL.revokeObjectURL;

beforeEach(() => {
  created = [];
  revoked = [];
  contentRequests = 0;
  let n = 0;
  URL.createObjectURL = vi.fn(() => {
    const u = `blob:preview-${n++}`;
    created.push(u);
    return u;
  });
  URL.revokeObjectURL = vi.fn((u: string) => {
    revoked.push(u);
  });
  server.use(
    http.get(`*/deployments/${DEPLOYMENT}/files/${KEY}/content`, () => {
      contentRequests += 1;
      return HttpResponse.arrayBuffer(new ArrayBuffer(8), {
        headers: { "Content-Type": "image/png" },
      });
    }),
  );
});

afterEach(() => {
  URL.createObjectURL = realCreateObjectURL;
  URL.revokeObjectURL = realRevokeObjectURL;
  vi.restoreAllMocks();
});

describe("useDeploymentFilePreview", () => {
  it("resolves an object URL for the file's bytes", async () => {
    const { result } = renderHook(
      () => useDeploymentFilePreview(DEPLOYMENT, KEY),
      { wrapper: createHookWrapper().wrapper },
    );
    await waitFor(() => expect(result.current).toBe("blob:preview-0"));
  });

  it("revokes the object URL and drops the cached blob on unmount", async () => {
    const { wrapper, queryClient } = createHookWrapper();
    const { result, unmount } = renderHook(
      () => useDeploymentFilePreview(DEPLOYMENT, KEY),
      { wrapper },
    );
    await waitFor(() => expect(result.current).toBe("blob:preview-0"));
    expect(
      queryClient.getQueryData<Blob>(fileKeys.content(DEPLOYMENT, KEY))?.size,
    ).toBe(8);

    unmount();

    expect(revoked).toContain("blob:preview-0");
    await waitFor(() =>
      expect(
        queryClient.getQueryData(fileKeys.content(DEPLOYMENT, KEY)),
      ).toBeUndefined(),
    );
  });

  it("renders nothing to preview when the fetch fails", async () => {
    server.use(
      http.get(`*/deployments/${DEPLOYMENT}/files/${KEY}/content`, () =>
        HttpResponse.text("nope", { status: 500 }),
      ),
    );
    const { result } = renderHook(
      () => useDeploymentFilePreview(DEPLOYMENT, KEY),
      { wrapper: createHookWrapper().wrapper },
    );
    await new Promise((r) => setTimeout(r, 50));
    expect(result.current).toBeUndefined();
    expect(created).toHaveLength(0);
  });

  it("survives the invalidation that upload and delete fire", async () => {
    const { wrapper, queryClient } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentFilePreview(DEPLOYMENT, KEY),
      { wrapper },
    );
    await waitFor(() => expect(result.current).toBe("blob:preview-0"));
    expect(contentRequests).toBe(1);

    await queryClient.invalidateQueries({
      queryKey: fileKeys.all(DEPLOYMENT),
    });
    await queryClient.invalidateQueries({
      queryKey: fileKeys.usage(DEPLOYMENT),
    });

    await new Promise((r) => setTimeout(r, 50));
    expect(contentRequests).toBe(1);
  });

  it("drops the preview when the file is deleted", async () => {
    let deleted = false;
    server.use(
      http.delete(`*/deployments/${DEPLOYMENT}/files/${KEY}`, () => {
        deleted = true;
        return HttpResponse.json({});
      }),
      http.get(`*/deployments/${DEPLOYMENT}/files/${KEY}/content`, () => {
        if (deleted) {
          return HttpResponse.arrayBuffer(new ArrayBuffer(0), { status: 404 });
        }
        contentRequests += 1;
        return HttpResponse.arrayBuffer(new ArrayBuffer(8), {
          headers: { "Content-Type": "image/png" },
        });
      }),
    );
    const { wrapper, queryClient } = createHookWrapper();
    const preview = renderHook(
      () => useDeploymentFilePreview(DEPLOYMENT, KEY),
      { wrapper },
    );
    const panel = renderHook(() => useDeleteDeploymentFile(DEPLOYMENT), {
      wrapper,
    });

    await waitFor(() => expect(preview.result.current).toBe("blob:preview-0"));
    expect(
      queryClient.getQueryData<Blob>(fileKeys.content(DEPLOYMENT, KEY))?.size,
    ).toBe(8);

    await panel.result.current.mutateAsync(KEY);

    await waitFor(() =>
      expect(
        queryClient.getQueryData(fileKeys.content(DEPLOYMENT, KEY)),
      ).toBeUndefined(),
    );
    await waitFor(() => expect(preview.result.current).toBeUndefined());
    expect(revoked).toContain("blob:preview-0");
  });
});
