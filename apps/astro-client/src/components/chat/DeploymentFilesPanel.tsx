import { useCallback, useRef, useState } from "react";
import { Download, FileIcon, Trash2, Upload } from "lucide-react";
import {
  useDeleteDeploymentFile,
  useDeploymentFiles,
  useDownloadDeploymentFile,
  useUploadDeploymentFile,
} from "@/api/queries/files";
import type { DeploymentFileMeta } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";
import { formatBytes, formatDateShort } from "@/lib/format-utils";
import { fileUploadErrorMessage } from "@/lib/chat/file-upload";
import { cn } from "@/lib/utils";

export function DeploymentFilesPanel({ deploymentId }: { deploymentId: string }) {
  const { data, isLoading, isError, error } = useDeploymentFiles(deploymentId);
  const uploadFile = useUploadDeploymentFile(deploymentId);
  const deleteFile = useDeleteDeploymentFile(deploymentId);
  const downloadFile = useDownloadDeploymentFile(deploymentId);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const [pendingDelete, setPendingDelete] = useState<DeploymentFileMeta | null>(
    null,
  );

  const onFilesSelected = useCallback(
    async (fileList: FileList | null) => {
      if (!fileList || fileList.length === 0) return;
      for (const file of Array.from(fileList)) {
        try {
          await uploadFile.mutateAsync(file);
        } catch {
          // Surfaced via the mutation error banner; continue with the rest.
        }
      }
      if (inputRef.current) inputRef.current.value = "";
    },
    [uploadFile],
  );

  const files = data?.files ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <p className="text-body-sm text-muted-foreground">
          Upload files for this agent to read, and download files it writes back.
          Stored on the deployment&apos;s disk at{" "}
          <span className="font-mono">/data/files</span>.
        </p>
        {/* Native <label> trigger: clicking it opens the file dialog with no
            JS, which is reliable across browsers where a programmatic
            input.click() on a display:none input can be a no-op. */}
        <Button
          asChild
          size="sm"
          className={cn(
            "w-full",
            uploadFile.isPending && "pointer-events-none opacity-60",
          )}
        >
          <label className="cursor-pointer">
            {uploadFile.isPending ? (
              <Spinner className="size-4" />
            ) : (
              <Upload className="size-4" />
            )}
            {uploadFile.isPending ? "Uploading..." : "Upload"}
            <input
              ref={inputRef}
              type="file"
              multiple
              className="sr-only"
              disabled={uploadFile.isPending}
              onChange={(e) => void onFilesSelected(e.target.files)}
            />
          </label>
        </Button>
        {uploadFile.isError && (
          <p className="text-body-sm text-destructive">
            {fileUploadErrorMessage(uploadFile.error, "Upload failed.")}
          </p>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <Spinner className="size-5" />
        </div>
      ) : isError ? (
        <p className="text-body-sm text-muted-foreground">
          {fileUploadErrorMessage(error, "Failed to load files.")}
        </p>
      ) : files.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-border bg-surface/60 px-4 py-8 text-center">
          <FileIcon className="size-5 text-muted-foreground" />
          <p className="text-body-sm text-muted-foreground">No files yet.</p>
        </div>
      ) : (
        <TooltipProvider delayDuration={0}>
          <div className="flex flex-col gap-1.5">
            {files.map((file) => (
              <div
                key={file.key}
                className="flex items-center gap-2 rounded-lg border border-border bg-surface/60 px-2.5 py-2"
              >
                <FileIcon className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-body-sm font-medium text-foreground">
                    {file.name}
                  </p>
                  <p className="text-label text-faint-foreground">
                    {formatBytes(file.size)} · {formatDateShort(file.updated_at)}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-0.5">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Download ${file.name}`}
                        disabled={downloadFile.isPending}
                        onClick={() =>
                          downloadFile.mutate({ key: file.key, name: file.name })
                        }
                      >
                        <Download className="size-4" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="bottom">Download</TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Delete ${file.name}`}
                        onClick={() => setPendingDelete(file)}
                      >
                        <Trash2 className="size-4 text-destructive" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="bottom">Delete</TooltipContent>
                  </Tooltip>
                </div>
              </div>
            ))}
          </div>
        </TooltipProvider>
      )}

      <ConfirmationDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
        title="Delete file"
        description={
          <>
            This will permanently delete{" "}
            <span className="font-semibold">{pendingDelete?.name}</span> from the
            deployment&apos;s disk.{" "}
            <span className="font-semibold text-destructive">
              This action cannot be undone.
            </span>
          </>
        }
        checkboxLabel="I understand this file will be permanently deleted."
        actionLabel="Delete file"
        pendingLabel="Deleting..."
        error={deleteFile.isError ? (deleteFile.error as Error) : null}
        defaultErrorMessage="Failed to delete file."
        isPending={deleteFile.isPending}
        canConfirm={pendingDelete !== null}
        onConfirm={() => {
          if (!pendingDelete) return;
          deleteFile.mutate(pendingDelete.key, {
            onSuccess: () => setPendingDelete(null),
          });
        }}
        onReset={() => deleteFile.reset()}
      />
    </div>
  );
}
