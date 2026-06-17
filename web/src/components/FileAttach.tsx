import { useRef, useState } from "react";
import { Paperclip, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { api, BackendUnreachable } from "@/lib/api";
import type { UploadedFile } from "@/lib/types";
import { cn } from "@/lib/utils";

const MAX_BYTES = 50 * 1024 * 1024;

const ALLOWED_EXT = [
  ".txt", ".md", ".csv", ".json",
  ".xlsx", ".xls", ".pdf", ".docx",
  ".png", ".jpg", ".jpeg",
];

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i === -1 ? "" : name.slice(i).toLowerCase();
}

export function humanSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export function FileAttach({
  onAdd,
  disabled,
  className,
  title = "Attach file",
}: {
  onAdd: (f: UploadedFile) => void;
  disabled?: boolean;
  className?: string;
  title?: string;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

  const handlePick = async (file: File) => {
    const ext = extOf(file.name);
    if (!ALLOWED_EXT.includes(ext)) {
      toast.error("Unsupported file type", {
        description: `Allowed: ${ALLOWED_EXT.join(", ")}`,
      });
      return;
    }
    if (file.size > MAX_BYTES) {
      toast.error("File too large", {
        description: `Max ${humanSize(MAX_BYTES)}; this file is ${humanSize(file.size)}.`,
      });
      return;
    }

    setUploading(true);
    try {
      const uploaded = await api.uploadFile(file);
      onAdd(uploaded);
      toast.success(`Attached ${file.name}`, {
        description: humanSize(uploaded.size),
      });
    } catch (e: any) {
      if (e instanceof BackendUnreachable) {
        toast.error("Backend unreachable", {
          description: "Start the Go server: `go run ./cmd/server` or `./kyoci-agent`, then retry.",
        });
      } else {
        toast.error("Upload failed", { description: e.message });
      }
    } finally {
      setUploading(false);
    }
  };

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          const picked = e.target.files;
          if (!picked) return;
          Array.from(picked).forEach(handlePick);
          e.target.value = "";
        }}
      />
      <button
        type="button"
        disabled={disabled || uploading}
        onClick={() => inputRef.current?.click()}
        title={title}
        aria-label={title}
        data-cursor="hover"
        className={cn(
          "inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border transition-all duration-200 ring-glow",
          "border-white/10 bg-white/5 text-[var(--color-ink-muted)] hover:bg-white/10 hover:text-[var(--color-ink)]",
          (disabled || uploading) && "opacity-40 cursor-not-allowed hover:bg-white/5",
          className,
        )}
      >
        {uploading ? (
          <Loader2 className="h-5 w-5 animate-spin" />
        ) : (
          <Paperclip className="h-5 w-5" />
        )}
      </button>
    </>
  );
}
